# Comprehensive Architecture Review
## Claude Code Switcher (ccs) - Deep Analysis

**Reviewer**: Senior Go Engineer
**Date**: 2025-10-29
**Scope**: Full codebase analysis covering architecture, code quality, edge cases, and security

---

## Executive Summary

**Overall Assessment**: ⭐⭐⭐⭐½ (4.5/5)

The codebase demonstrates solid engineering practices with excellent testability, clean separation of concerns, and good use of Go idioms. There are **critical atomicity and security issues** around file operations that need immediate attention.

**Critical Issues**: 3 | **High Priority**: 5 | **Medium Priority**: 8 | **Low Priority**: 6

---

## 1. Architecture Analysis

### ✅ Strengths

1. **Dependency Injection Excellence**
   - Filesystem abstraction (`afero.Fs`) enables hermetic testing
   - Time injection (`now func()`) allows deterministic time-based tests
   - Interface-based design (`Prompter`) supports test doubles

2. **Separation of Concerns**
   - Clean boundary between CLI (`internal/cli`) and domain (`internal/ccs`)
   - Command construction separated from business logic
   - Path management centralized in `paths.go`

3. **Testability First**
   - 80% coverage threshold enforced in CI
   - Uses `afero.NewMemMapFs()` for isolated unit tests
   - Table-driven tests for validation logic

### ⚠️ Architectural Concerns

#### **Manager God Object Anti-Pattern** (manager.go)
```go
// Manager does too much:
// - File backup operations
// - Settings persistence
// - Name validation
// - MD5 calculation
// - Directory management
// - State tracking
```

**Impact**: Violates Single Responsibility Principle, makes testing complex, reduces modularity

**Recommendation**: Decompose into focused components:
```
ccs/
  ├── backup/      # Backup operations & content-addressing
  ├── settings/    # Settings CRUD operations
  ├── validator/   # Name validation
  └── storage/     # Low-level file operations
```

#### **Architecture Gap**: Clean Architecture Incomplete
AGENTS.md mentions "internal/core, internal/interface, and internal/infrastructure are reserved for future layering" but these don't exist yet. Current structure mixes domain logic with infrastructure concerns.

---

## 2. Critical Security Issues 🔴

### **SEC-1: World-Readable File Permissions** (manager.go:46, 109, 197)

**Current**:
```go
fs.MkdirAll(p, 0o755)  // drwxr-xr-x - world can read
afero.WriteFile(path, data, 0o644)  // -rw-r--r-- - world can read
```

**Problem**: Settings files may contain API keys, tokens, or other secrets. Current permissions allow any user on the system to read these files.

**Fix**:
```go
// Directories
fs.MkdirAll(p, 0o700)  // drwx------ - owner only

// Files
afero.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)  // -rw------- - owner only
```

**Locations to change**:
- `manager.go:46` - `InitInfra()`
- `manager.go:109` - `backupFile()`
- `manager.go:193` - `copyFile()`
- `manager.go:144` - `SetActiveSettings()`

### **SEC-2: Symlink Attack Vulnerability**

**Location**: All file operations in `manager.go`

**Attack Scenario**:
```bash
# Attacker creates symlink
ln -s /etc/passwd ~/.claude/settings.json

# User runs: ccs save
# -> Overwrites /etc/passwd with settings content
```

**Current Code** has no symlink validation:
```go
func (m *Manager) Use(name string) error {
    // No check if activeSettingsPath() is a symlink!
    if err := m.copyFile(targetPath, m.activeSettingsPath()); err != nil {
        return err
    }
}
```

**Fix**: Add validation helper:
```go
// Add to manager.go
func (m *Manager) validatePathSafety(path string) error {
    info, err := m.fs.Lstat(path)  // Use Lstat, not Stat
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil  // Non-existent is fine
        }
        return fmt.Errorf("failed to check path: %w", err)
    }

    if info.Mode()&os.ModeSymlink != 0 {
        return fmt.Errorf("refusing to operate on symlink: %s", path)
    }
    return nil
}
```

**Apply to**:
- `Use()` - before copying to active path
- `Save()` - before writing to stored path
- `copyFile()` - before writing destination

### **SEC-3: MD5 Collision Vulnerability** (manager.go:53-77)

**Problem**: MD5 is cryptographically broken. While collision attacks are unlikely for this use case, SHA-256 provides better security guarantees without performance penalty.

**Collision Risk Example**:
- Two different settings files could hash to same MD5
- Second backup would overwrite first (line 100-104)
- Original settings lost without user awareness

**Fix**:
```go
import "crypto/sha256"

func (m *Manager) CalculateHash(path string) (string, error) {
    info, err := m.fs.Stat(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return "", nil
        }
        return "", fmt.Errorf("failed to stat file for hashing: %w", err)
    }
    if info.Size() == 0 {
        return "", nil
    }

    f, err := m.fs.Open(path)
    if err != nil {
        return "", fmt.Errorf("failed to open file for hashing: %w", err)
    }
    defer f.Close()

    h := sha256.New()  // Changed from md5.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", fmt.Errorf("failed to hash file: %w", err)
    }
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

**Migration**: Rename `CalculateMD5` → `CalculateHash` across codebase

---

## 3. Critical Correctness Issues 🔴

### **BUG-1: Non-Atomic File Replacement** (manager.go:213-217)

**Current Code**:
```go
// Line 213: Remove destination first
if err := m.fs.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
    return err  // ❌ If this fails, file might be partially deleted
}

// Line 217: Then rename
return m.fs.Rename(tmp, dst)  // ❌ If this fails, dst is already gone!
```

**Failure Scenario**:
```
1. Remove(dst) succeeds
2. Rename(tmp, dst) fails due to disk full / permissions / etc
3. User's active settings.json is GONE
4. No way to recover
```

**Fix**: Unix `rename()` is **already atomic** and overwrites destination:
```go
// REMOVE the entire Remove() block:
// if err := m.fs.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
//     return err
// }

// Just do atomic rename:
return m.fs.Rename(tmp, dst)  // ✅ Atomically replaces dst
```

This is a **one-line fix** that eliminates the data loss window entirely.

---

## 4. High-Priority Issues

### **DATA-1: Empty File Backup Skipped Silently** (manager.go:62-63, 85-86)

```go
if info.Size() == 0 {
    return "", nil  // ❌ Returns empty hash
}

// Later...
if md5sum == "" {
    return nil  // ❌ Silently skips backup
}
```

**Problem**: User accidentally truncates `settings.json`:
```bash
echo "" > ~/.claude/settings.json  # Oops!
ccs save work  # ❌ No backup created!
```

**Current Behavior**: Silent skip - no warning, no error
**User Expectation**: Either backup or clear error message

**Recommendation** (pick one):
```go
// Option 1: Backup empty files (hash of empty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
if info.Size() == 0 {
    return "empty", nil  // Special marker
}

// Option 2: Return error
if info.Size() == 0 {
    return "", errors.New("refusing to backup empty settings file")
}

// Option 3: Log warning (requires adding logging)
if info.Size() == 0 {
    log.Warn("skipping backup of empty file")
    return "", nil
}
```

### **ERR-1: Resource Leak - Inconsistent Close() Handling** (manager.go:96, 115)

```go
defer source.Close()  // ❌ Error ignored

// Later:
closeErr := dest.Close()
if closeErr != nil {
    m.fs.Remove(backupPath)
    return fmt.Errorf("failed to close backup: %w", closeErr)
}
```

**Problem**: Inconsistent - deferred closes ignore errors, explicit closes check them.

**Best Practice**: Either check all or check none. For file writes, checking is important because `Close()` can flush buffers and fail.

**Fix**: Use named return:
```go
func (m *Manager) backupFile(path string) (err error) {
    source, err := m.fs.Open(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil
        }
        return fmt.Errorf("failed to open file for backup: %w", err)
    }
    defer func() {
        if cerr := source.Close(); cerr != nil && err == nil {
            err = fmt.Errorf("failed to close source: %w", cerr)
        }
    }()
    // ... rest of function
}
```

### **VAL-1: Path Traversal Validation Could Be Stronger** (manager.go:148-171)

**Current Validation**:
```go
// ✅ Checks for invalid chars including "/"
if invalidCharsPattern.MatchString(trimmed) {
    return false, errNameInvalidChars
}
```

**Gap**: Pattern is `[<>:"/\\|?*]` which catches most issues, but doesn't explicitly check:
- Null bytes (`\x00`)
- Relative path components in compound names

**Recommendation**: Add explicit checks for defense-in-depth:
```go
func (m *Manager) ValidateSettingsName(name string) (bool, error) {
    trimmed := strings.TrimSpace(name)
    if len(trimmed) == 0 {
        return false, errNameEmpty
    }
    if trimmed == "." || trimmed == ".." {
        return false, errNameDot
    }

    // ✨ Add: explicit null byte check
    if strings.ContainsRune(trimmed, 0) {
        return false, errors.New("name contains null byte")
    }

    // Existing checks...
    for _, r := range trimmed {
        if r < 0x20 || r > 0x7e {
            return false, errNameNonPrintable
        }
        if r == 0x7f {
            return false, errNameNonPrintable
        }
    }
    if invalidCharsPattern.MatchString(trimmed) {
        return false, errNameInvalidChars
    }
    if reservedNamePattern.MatchString(trimmed) {
        return false, errNameReserved
    }
    return true, nil
}
```

### **VAL-2: Missing Input Validation on Command Arguments**

**Location**: `command.go:68-94`

```go
func newUseCommand(mgr *ccs.Manager, prompter Prompter, stdout io.Writer) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "use [name]",
        RunE: func(cmd *cobra.Command, args []string) error {
            name := ""
            if len(args) > 0 {
                name = args[0]  // ❌ No validation!
            }
            // ...
            if err := mgr.Use(name); err != nil {
                return err
            }
```

**Problem**: User can pass invalid names directly:
```bash
ccs use "../../../etc/passwd"  # Gets to Manager.Use() before validation
```

**Fix**: Validate early:
```go
if len(args) > 0 {
    name = args[0]
    // ✨ Validate immediately
    if valid, err := mgr.ValidateSettingsName(name); !valid {
        return fmt.Errorf("invalid settings name: %w", err)
    }
}
```

### **TEST-1: Missing Integration Tests**

**Current**: Only unit tests with mocks

**Missing**: End-to-end CLI tests that actually run the binary:
```go
func TestCLIWorkflow(t *testing.T) {
    // Build binary first
    cmd := exec.Command("go", "build", "-o", "./ccs-test", "./cmd/ccs")
    if err := cmd.Run(); err != nil {
        t.Fatalf("build failed: %v", err)
    }
    defer os.Remove("./ccs-test")

    // Test list command
    cmd = exec.Command("./ccs-test", "list")
    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("list failed: %v", err)
    }

    // Assert output format
    if !strings.Contains(string(output), "No saved settings") {
        t.Errorf("unexpected output: %s", output)
    }
}
```

---

## 5. Code Quality Issues

### **STYLE-1: Inconsistent Error Wrapping**

**Good Example** (manager.go:47):
```go
return fmt.Errorf("failed to create directory %s: %w", p, err)
```

**Bad Example** (command.go:79):
```go
return errors.New("no stored settings available")  // ❌ No context
```

**Recommendation**: Always wrap with context:
```go
return fmt.Errorf("use command: no stored settings available in %s", mgr.SettingsStoreDir())
```

### **STYLE-2: Magic Numbers** (promptui_prompter.go:46)

```go
Size: 10,  // ❌ What does 10 mean?
```

**Fix**:
```go
const defaultMenuSize = 10  // Number of items visible in selection menu

Size: defaultMenuSize,
```

### **STYLE-3: Complex Slice Manipulation** (command.go:281)

```go
reordered := append([]string{defaultValue}, append(append([]string{}, items[:idx]...), items[idx+1:]...)...)
```

**This is hard to read!** Triple-nested appends make logic unclear.

**Fix**:
```go
func reorderWithDefault(items []string, defaultValue string) []string {
    if defaultValue == "" {
        return items
    }
    idx := -1
    for i, item := range items {
        if item == defaultValue {
            idx = i
            break
        }
    }
    if idx <= 0 {
        return items
    }

    // Clear, readable reordering
    reordered := make([]string, 0, len(items))
    reordered = append(reordered, defaultValue)
    reordered = append(reordered, items[:idx]...)
    reordered = append(reordered, items[idx+1:]...)
    return reordered
}
```

### **STYLE-4: Error Variables Should Be Prefixed with 'Err'** (manager.go:19-25)

**Current**:
```go
var (
    errNameEmpty        = errors.New("Name cannot be empty.")
    errNameDot          = errors.New("Name cannot be '.' or '..'.")
    errNameNonPrintable = errors.New("Name contains non-printable ASCII characters.")
    errNameInvalidChars = errors.New("Name contains invalid characters (<>:\"/|?*).")
    errNameReserved     = errors.New("Name is a reserved system filename.")
)
```

**Problem**: Common Go convention is `ErrName` for exported, `errName` for unexported. These follow convention but could be more descriptive.

**Better**:
```go
var (
    ErrSettingsNameEmpty        = errors.New("settings name cannot be empty")
    ErrSettingsNameDot          = errors.New("settings name cannot be '.' or '..'")
    ErrSettingsNameNonPrintable = errors.New("settings name contains non-printable characters")
    ErrSettingsNameInvalidChars = errors.New("settings name contains invalid characters (<>:\"/|?*)")
    ErrSettingsNameReserved     = errors.New("settings name is a reserved system filename")
)
```

Export them so callers can do `errors.Is(err, ccs.ErrSettingsNameEmpty)`.

---

## 6. Testing Gaps

### **TEST-2: No Boundary Testing for Validation**

**Current** (manager_test.go:115):
```go
invalids := []string{"my/settings", "my设置", "CON", " ", ".."}
```

**Missing Edge Cases**:
```go
func TestValidateSettingsNameBoundaries(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"max length 255", strings.Repeat("a", 255), false},
        {"over max 256", strings.Repeat("a", 256), true},  // Might hit filesystem limit
        {"null byte", "test\x00file", true},
        {"control chars", "test\x01\x02", true},
        {"unicode normalization NFC", "café", true},  // é as single char
        {"unicode normalization NFD", "café", true},  // e + combining accent
        {"emoji", "settings😀", true},
        {"rtl override", "test\u202e", true},  // Right-to-left override attack
        {"mixed scripts", "testтест", true},   // Latin + Cyrillic
        {"windows reserved COM1", "COM1", true},
        {"windows reserved lowercase", "con", true},
    }

    mgr := newTestManager(t)
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            valid, err := mgr.ValidateSettingsName(tt.input)
            if tt.wantErr && (valid || err == nil) {
                t.Errorf("expected error for %q", tt.input)
            }
            if !tt.wantErr && (!valid || err != nil) {
                t.Errorf("unexpected error for %q: %v", tt.input, err)
            }
        })
    }
}
```

### **TEST-3: No Error Path Testing**

**Current**: Tests focus on happy paths

**Missing**: Error injection tests:
```go
func TestCopyFileDestinationReadOnly(t *testing.T) {
    mgr := newTestManager(t)

    // Create read-only destination
    roFs := afero.NewReadOnlyFs(afero.NewMemMapFs())
    mgr.fs = afero.NewCopyOnWriteFs(roFs, afero.NewMemMapFs())

    src := filepath.Join(mgr.claudeDir(), "src.json")
    dst := filepath.Join(mgr.claudeDir(), "dst.json")

    if err := afero.WriteFile(mgr.fs, src, []byte("data"), 0o600); err != nil {
        t.Fatalf("setup: %v", err)
    }

    err := mgr.copyFile(src, dst)
    if err == nil {
        t.Fatal("expected error for read-only filesystem")
    }
}
```

---

## 7. Documentation Issues

### **DOC-1: Missing godoc Comments**

**Example** (manager.go:220):
```go
// ❌ No comment
func (m *Manager) Use(name string) error {
```

**Should be**:
```go
// Use activates the specified settings profile by copying it to the active
// settings location. It backs up the current settings before overwriting.
// The profile name must exist in the settings store and pass validation.
//
// The operation is atomic - if it fails, the current settings remain unchanged.
//
// Returns an error if:
//   - The profile name is invalid (see ValidateSettingsName)
//   - The profile doesn't exist in the settings store
//   - File operations fail (permissions, disk space, etc.)
//
// Example:
//   err := mgr.Use("work")
//   if err != nil {
//       log.Fatal(err)
//   }
func (m *Manager) Use(name string) error {
```

### **DOC-2: Backup Semantics Undocumented**

The content-addressed backup system is clever but undocumented:
```go
// ❌ Comment doesn't explain deduplication
// backupFile copies the provided file into the backup directory.
func (m *Manager) backupFile(path string) (err error) {
```

**Should explain**:
```go
// backupFile creates a content-addressed backup of the file at path.
//
// The backup uses SHA-256 hash as filename, enabling deduplication:
//   - Identical content reuses the same backup file
//   - Modified time (mtime) is updated on each backup event
//   - Empty files are not backed up
//   - Missing files are silently skipped
//
// Backup files are stored in ~/.claude/switch-settings-backup/ as:
//   <sha256-hash>.json
//
// This approach ensures:
//   - Multiple backups of identical content don't waste space
//   - The prune command can use mtime to determine backup age
//   - Each unique settings version is preserved exactly once
func (m *Manager) backupFile(path string) (err error) {
```

### **DOC-3: README Missing Security Considerations**

**Current README**: Focuses on usage, doesn't mention security

**Add Section**:
```markdown
## Security

### File Permissions

All settings files and directories are created with restrictive permissions:
- Directories: `0700` (owner read/write/execute only)
- Files: `0600` (owner read/write only)

This prevents other users on multi-user systems from reading your settings,
which may contain API keys, tokens, or other sensitive data.

### Symlink Protection

`ccs` validates that target paths are not symbolic links before writing,
preventing symlink attacks that could overwrite arbitrary files.

### Backup Security

Backups in `~/.claude/switch-settings-backup/` use SHA-256 content addressing.
While backups themselves aren't encrypted, they inherit the same restrictive
file permissions as settings files.
```

---

## 8. Recommendations by Priority

### 🔴 **Critical (Must Fix Before v1.0)**

1. ✅ **Fix non-atomic file replacement** (BUG-1)
   - Location: `manager.go:213-217`
   - Fix: Remove `m.fs.Remove(dst)` call
   - Impact: Prevents data loss during file operations
   - Effort: 5 minutes

2. ✅ **Add file permissions 0o600/0o700** (SEC-1)
   - Locations: Multiple files
   - Fix: Change all `0o644`→`0o600`, `0o755`→`0o700`
   - Impact: Prevents unauthorized access to sensitive settings
   - Effort: 30 minutes

3. ✅ **Add symlink validation** (SEC-2)
   - Location: `manager.go` - add `validatePathSafety()`
   - Fix: Use `Lstat()` before file operations
   - Impact: Prevents symlink attacks
   - Effort: 1-2 hours

### 🟠 **High Priority (Should Fix Soon)**

4. ⚠️ **Handle empty file backups explicitly** (DATA-1)
   - Location: `manager.go:62-63, 85-86`
   - Fix: Error or log warning instead of silent skip
   - Impact: Better user experience and data safety
   - Effort: 30 minutes

5. ⚠️ **Fix resource leaks in Close()** (ERR-1)
   - Location: `manager.go:96, 115`
   - Fix: Use named return to capture deferred close errors
   - Impact: Prevents silent corruption from failed buffer flushes
   - Effort: 1 hour

6. ⚠️ **Validate command arguments early** (VAL-2)
   - Location: `command.go:68-94`
   - Fix: Call `ValidateSettingsName()` before passing to Manager
   - Impact: Better error messages, defense in depth
   - Effort: 30 minutes

7. ⚠️ **Switch MD5 → SHA256** (SEC-3)
   - Location: `manager.go:53-77`
   - Fix: Change `md5.New()` to `sha256.New()`
   - Impact: Eliminates collision risk, future-proof
   - Effort: 30 minutes + migration testing

8. ⚠️ **Add godoc comments** (DOC-1)
   - Location: All exported functions
   - Fix: Add comprehensive documentation
   - Impact: Better maintainability and user experience
   - Effort: 2-3 hours

### 🟡 **Medium Priority (Nice to Have)**

9. 📝 Strengthen path validation (VAL-1)
10. 📝 Add integration tests (TEST-1)
11. 📝 Improve error message consistency (STYLE-1)
12. 📝 Extract magic numbers to constants (STYLE-2)
13. 📝 Simplify complex slice operations (STYLE-3)
14. 📝 Export error variables (STYLE-4)
15. 📝 Add boundary testing (TEST-2)
16. 📝 Document backup semantics (DOC-2)

### 🟢 **Low Priority (Future)**

17. 💡 Break Manager into smaller components (architecture)
18. 💡 Add structured logging (zerolog/slog)
19. 💡 Add error path testing (TEST-3)
20. 💡 Add README security section (DOC-3)
21. 💡 Add version field to saved profiles
22. 💡 Implement clean architecture layers

---

## 9. Specific Code Improvements

### Example Fix: Atomic File Replacement + Symlink Protection

**Before** (manager.go:184-218):
```go
func (m *Manager) copyFile(src, dst string) error {
    source, err := m.fs.Open(src)
    if err != nil {
        return err
    }
    defer source.Close()

    dir := filepath.Dir(dst)
    if err := m.fs.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    tmp := dst + ".tmp"
    dest, err := m.fs.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
    if err != nil {
        return err
    }
    _, copyErr := io.Copy(dest, source)
    closeErr := dest.Close()

    if copyErr != nil {
        m.fs.Remove(tmp)
        return copyErr
    }
    if closeErr != nil {
        m.fs.Remove(tmp)
        return closeErr
    }

    // ❌ This is the problem:
    if err := m.fs.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
        return err
    }

    return m.fs.Rename(tmp, dst)
}
```

**After**:
```go
func (m *Manager) copyFile(src, dst string) (err error) {
    // ✅ Validate source and destination aren't symlinks
    if err := m.validatePathSafety(src); err != nil {
        return fmt.Errorf("validate source: %w", err)
    }
    if err := m.validatePathSafety(dst); err != nil {
        return fmt.Errorf("validate destination: %w", err)
    }

    source, err := m.fs.Open(src)
    if err != nil {
        return fmt.Errorf("open source: %w", err)
    }
    defer func() {
        if cerr := source.Close(); cerr != nil && err == nil {
            err = fmt.Errorf("close source: %w", cerr)
        }
    }()

    dir := filepath.Dir(dst)
    if err := m.fs.MkdirAll(dir, 0o700); err != nil {  // ✅ Secure perms
        return fmt.Errorf("create directory: %w", err)
    }

    // Create temp file in same directory (enables atomic rename)
    tmp := dst + ".tmp"
    dest, err := m.fs.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)  // ✅ Secure perms
    if err != nil {
        return fmt.Errorf("create temp file: %w", err)
    }

    // Copy with proper error handling
    _, copyErr := io.Copy(dest, source)
    closeErr := dest.Close()

    if copyErr != nil || closeErr != nil {
        m.fs.Remove(tmp)
        if copyErr != nil {
            return fmt.Errorf("copy data: %w", copyErr)
        }
        return fmt.Errorf("close temp file: %w", closeErr)
    }

    // ✅ Atomic rename (no Remove needed!)
    // Unix rename() atomically replaces destination
    if err := m.fs.Rename(tmp, dst); err != nil {
        m.fs.Remove(tmp)
        return fmt.Errorf("atomic rename: %w", err)
    }

    return nil
}

// ✅ New helper function
func (m *Manager) validatePathSafety(path string) error {
    info, err := m.fs.Lstat(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil  // Non-existent is fine
        }
        return fmt.Errorf("failed to check path: %w", err)
    }

    if info.Mode()&os.ModeSymlink != 0 {
        return fmt.Errorf("refusing to operate on symlink: %s", path)
    }
    return nil
}
```

**Changes**:
1. ✅ Removed `m.fs.Remove(dst)` - atomic rename handles overwrite
2. ✅ Added symlink validation before operations
3. ✅ Changed permissions to `0o700`/`0o600`
4. ✅ Fixed deferred Close() error handling
5. ✅ Improved error messages with context

---

## 10. Final Assessment

### What This Codebase Does Well

1. ✅ **Testing Excellence**: 80% coverage, hermetic tests, table-driven
2. ✅ **Dependency Injection**: Properly abstracted for testability
3. ✅ **Error Handling**: Consistent wrapping (mostly), sentinel errors
4. ✅ **Code Organization**: Clean package structure
5. ✅ **Go Idioms**: Follows standard patterns

### Critical Improvements Needed

1. 🔴 **Atomicity** - Fix non-atomic file operations (5 min)
2. 🔴 **Security hardening** - File permissions + symlink protection (2 hours)
3. 🟠 **Error handling** - Close() resource leaks (1 hour)
4. 🟠 **Hash algorithm** - MD5 → SHA256 (30 min)

### Architecture Evolution Path

**Current State**: Functional monolith with good testing

**Recommended Next Step** (after critical fixes):
```
internal/
  ├── ccs/
  │   ├── manager.go      # Orchestration only
  │   ├── backup/         # ✨ Extract backup logic
  │   ├── settings/       # ✨ Extract settings CRUD
  │   └── validator/      # ✨ Extract validation
  └── cli/                # Unchanged
```

**Long-term**: Keep current architecture unless adding features like:
- Cloud sync
- Settings diffing / merging
- Team profiles
- Encryption

For a CLI tool with narrow scope, current architecture is appropriate.

---

## Conclusion

This is a **well-structured codebase** with strong fundamentals. The critical issues identified are **easily fixable** with targeted changes. Priority should be:

1. **Atomicity first**: Fix file replacement (5 minutes - trivial change)
2. **Security**: Fix permissions and add symlink protection (2 hours)
3. **Robustness**: Handle edge cases explicitly (2-3 hours)
4. **Documentation**: Add godoc comments (2-3 hours)

**Estimated Total Effort**: 1 day for critical + high priority fixes

**Risk Assessment**: Low (issues are well-understood, fixes are straightforward)

The team should be proud of the testing infrastructure, clean code, and thoughtful design. Address the critical atomicity and security issues, and this will be production-ready.

**Recommendation**: Fix BUG-1 (non-atomic rename) immediately - it's a 1-line change that eliminates data loss risk.
