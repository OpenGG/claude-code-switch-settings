# Testing Best Practices

## Philosophy

**Test Quality > Coverage Numbers**

We maintain ~80% test coverage, but only with tests that provide real value. Coverage is a byproduct of good testing, not the goal itself.

### What Makes a Good Test?

A good test verifies one of these:
1. **Security requirements** - Prevents attacks
2. **Complex business logic** - Proves algorithms work correctly
3. **Integration workflows** - Validates user-facing operations
4. **Realistic error scenarios** - Handles failures gracefully

### What Makes a Bad Test?

A bad test:
- Tests third-party libraries (afero, Go stdlib)
- Tests trivial getters/setters with no logic
- Tests the Go compiler
- Requires complex mocking for no benefit
- Exists only to inflate coverage numbers

## Testing Priorities

### Priority 1: Security (90%+ coverage required)

The `validator` package is our attack surface. Every test prevents a vulnerability:

```go
// CRITICAL: Path traversal attack prevention
func TestValidateName_DotNavigation(t *testing.T) {
    tests := []string{".", "..", " . ", " .. "}
    // These could allow "../../../etc/passwd"
}

// CRITICAL: Null byte injection prevention
func TestValidateName_NullBytes(t *testing.T) {
    // "test\x00.json" might bypass extension checks
}
```

**Coverage target: >90%**

Test cases:
- ✅ Path traversal (., .., /, \)
- ✅ Null bytes (\x00)
- ✅ Control characters (tabs, newlines, ANSI escapes)
- ✅ Windows reserved names (CON, PRN, AUX, COM1-9, LPT1-9)
- ✅ Invalid filesystem chars (<>:"/\|?*)
- ✅ Unicode attacks (emoji, Chinese, Cyrillic)

### Priority 2: Complex Business Logic (80%+ coverage)

Functions with algorithms, state machines, or non-trivial logic:

```go
// COMPLEX: Content-addressed deduplication
func TestBackupFile_DeduplicationCreatesOnlyOneFile(t *testing.T) {
    // Same content = same SHA-256 hash = single backup file
    // This prevents storage waste
}

// COMPLEX: State machine with 5 possible states
func TestListEntries_ActiveModified(t *testing.T) {
    // Active profile with local modifications (hash mismatch)
    // User needs to know their changes aren't saved
}
```

**Coverage target: 80%+**

Examples:
- `backup/BackupFile` - Deduplication logic
- `backup/PruneBackups` - Time-based cleanup
- `settings/ListEntries` - State detection (5 states)
- `settings/ListStored` - Filtering and sorting

### Priority 3: Integration Tests (Full workflows)

Test complete user journeys through the Manager:

```go
func TestUseSwitchesSettingsAndUpdatesTimestamp(t *testing.T) {
    mgr := newTestManager(t)

    // Create stored profile
    afero.WriteFile(mgr.FileSystem(), "store/work.json", []byte("stored"))

    // Switch to it
    mgr.Use("work")

    // Verify active settings updated
    // Verify backup created
    // Verify timestamp updated
}
```

**Coverage: All user-facing operations**

### Priority 4: Don't Test (0% coverage acceptable)

**Simple wrappers** (already covered by integration):
```go
// DON'T test - one line, no logic
func (s *Storage) ReadFile(path string) ([]byte, error) {
    return afero.ReadFile(s.fs, path)
}
```

**Trivial getters**:
```go
// DON'T test - covered by integration
func (s *Service) BackupDir() string {
    return s.backupDir
}
```

**Defensive error handling**:
```go
// DON'T test - can't reliably trigger with MemMapFs
defer func() {
    if cerr := source.Close(); cerr != nil && err == nil {
        err = fmt.Errorf("close: %w", cerr)
    }
}()
```

## Test Organization

### File Structure

```
internal/ccs/
├── manager_test.go        # Integration tests (full workflows)
├── storage/
│   ├── storage.go
│   └── storage_test.go    # Atomic operations, security
├── backup/
│   ├── service.go
│   └── service_test.go    # Deduplication, pruning logic
├── settings/
│   ├── service.go
│   └── service_test.go    # State machine tests
└── validator/
    ├── validator.go
    └── validator_test.go  # SECURITY - comprehensive
```

### Naming Convention

**Pattern:** `TestFunctionName_Scenario`

```go
TestCopyFile_Success                    // Happy path
TestCopyFile_MissingSource              // Error case
TestCopyFile_SecurePermissions          // Security requirement
TestValidateName_NullBytes              // Attack prevention
TestListEntries_ActiveModified          // State combination
```

### Test Helpers

Use the `t.Helper()` pattern for reusable setup:

```go
func newTestService(t *testing.T) (*Service, afero.Fs) {
    t.Helper()
    fs := afero.NewMemMapFs()
    storage := storage.New(fs)

    // Setup directories
    fs.MkdirAll("/store", 0o700)
    fs.MkdirAll("/state", 0o700)

    return New(storage, "/store", "/state"), fs
}
```

### Table-Driven Tests

For validation logic with many cases:

```go
func TestValidateName_InvalidChars(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        char    string
    }{
        {"forward slash", "my/settings", "/"},
        {"backslash", "my\\settings", "\\"},
        {"colon", "my:settings", ":"},
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            valid, err := v.ValidateName(tt.input)
            if valid {
                t.Errorf("expected invalid for %s", tt.char)
            }
        })
    }
}
```

## Coverage Measurement

### Check Overall Coverage

```bash
go test -coverpkg=./internal/ccs/... -coverprofile=coverage.out ./internal/ccs/...
go tool cover -func=coverage.out | tail -1
```

**Target: ~80%** (flexible based on meaningful tests)

### Check Per-Package Coverage

```bash
go test -coverpkg=./internal/ccs/... -coverprofile=coverage.out ./internal/ccs/...
go tool cover -func=coverage.out | grep "validator/validator.go"
```

**Expected:**
- `validator/validator.go` → 90%+
- `backup/service.go` → 65%+
- `storage/storage.go` → 60%+
- `settings/service.go` → 90%+

### Generate HTML Report

```bash
go test -coverpkg=./internal/ccs/... -coverprofile=coverage.out ./internal/ccs/...
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

Look for:
- ✅ Green (covered) on security checks
- ✅ Green on complex business logic
- ⚠️ Red on defensive error handlers (acceptable)
- ⚠️ Red on simple wrappers (acceptable if integration tested)

## Examples of Good Tests

### Security Test (validator)

```go
func TestValidateName_PathTraversal(t *testing.T) {
    v := New()
    attacks := []string{
        "../etc/passwd",
        "../../secrets",
        "..",
        ".",
    }

    for _, attack := range attacks {
        valid, err := v.ValidateName(attack)
        if valid {
            t.Errorf("SECURITY: path traversal not blocked: %q", attack)
        }
        if !errors.Is(err, domain.ErrSettingsNameDot) &&
           !errors.Is(err, domain.ErrSettingsNameInvalidChars) {
            t.Errorf("wrong error for %q: %v", attack, err)
        }
    }
}
```

### Business Logic Test (backup)

```go
func TestBackupFile_DeduplicationCreatesOnlyOneFile(t *testing.T) {
    svc, fs := newTestService(t)
    path := "/test/file.json"

    // Write same content
    afero.WriteFile(fs, path, []byte("same content"), 0o644)

    // Backup twice
    svc.BackupFile(path)
    svc.BackupFile(path)

    // Should only have ONE backup (deduplicated by hash)
    entries, _ := afero.ReadDir(fs, svc.BackupDir())
    if len(entries) != 1 {
        t.Errorf("expected 1 backup (deduplicated), got %d", len(entries))
    }
}
```

### Integration Test (manager)

```go
func TestUseSwitchesSettings(t *testing.T) {
    mgr := newTestManager(t)
    store := mgr.SettingsStoreDir()

    // Setup: stored profile + current settings
    afero.WriteFile(mgr.FileSystem(), filepath.Join(store, "work.json"), []byte("stored"))
    afero.WriteFile(mgr.FileSystem(), mgr.ActiveSettingsPath(), []byte("current"))

    // Action: switch profiles
    err := mgr.Use("work")

    // Verify:
    // 1. No errors
    if err != nil {
        t.Fatalf("Use failed: %v", err)
    }

    // 2. Active settings replaced
    content, _ := afero.ReadFile(mgr.FileSystem(), mgr.ActiveSettingsPath())
    if string(content) != "stored" {
        t.Errorf("settings not switched")
    }

    // 3. Active name updated
    if mgr.GetActiveSettingsName() != "work" {
        t.Errorf("active name not updated")
    }

    // 4. Backup created for old settings
    backups, _ := afero.ReadDir(mgr.FileSystem(), mgr.BackupDir())
    if len(backups) == 0 {
        t.Errorf("backup not created")
    }
}
```

## When NOT to Write Tests

### Example 1: Simple Wrapper

```go
// storage/storage.go
func (s *Storage) Exists(path string) (bool, error) {
    return afero.Exists(s.fs, path)
}

// ❌ DON'T write: TestExists_True, TestExists_False
// ✅ DO: Use in integration tests (already covered)
```

### Example 2: Trivial Getter

```go
// backup/service.go
func (s *Service) BackupDir() string {
    return s.backupDir
}

// ❌ DON'T write: TestBackupDir_ReturnsCorrectPath
// ✅ DO: Verify it works in integration tests
```

### Example 3: Simple Setter

```go
// settings/service.go
func (s *Service) SetActiveName(name string) error {
    return s.storage.WriteFile(s.activeState, []byte(name))
}

// ❌ DON'T write: TestSetActiveName, TestSetActiveName_Overwrites
// ✅ DO: Use in integration tests (Save, Use workflows)
```

## Coverage Philosophy

### Good Coverage (79.1%)

```
✅ validator/ValidateName: 94.1% - Security checks comprehensive
✅ settings/ListEntries: 88.9% - State machine tested
✅ backup/PruneBackups: 88.2% - Time logic verified
✅ settings/ListStored: 91.7% - Business rules tested
⚠️ storage/CopyFile: 60.7% - Atomic ops tested, defensive errors not
⚠️ backup/BackupFile: 64.9% - Dedup tested, I/O errors not
```

**Total: 79.1%** with 45 meaningful tests

### Bad Coverage (81.6% - if we hadn't cleaned up)

```
✅ validator/ValidateName: 94.1% - Good!
✅ storage/ReadFile: 100% - Testing afero library ❌
✅ storage/WriteFile: 100% - Testing afero library ❌
✅ settings/GetActiveName: 100% - Testing trivial getter ❌
✅ settings/SetActiveName: 100% - Testing trivial setter ❌
```

**Total: 81.6%** with 60 tests (15 of them useless)

## Summary

### DO Write Tests For:
- ✅ Security validators (attack prevention)
- ✅ Complex algorithms (deduplication, hashing, pruning)
- ✅ State machines (ListEntries with 5 states)
- ✅ Business rules (file filtering, sorting)
- ✅ Integration workflows (Use → Save → List)
- ✅ Realistic error scenarios

### DON'T Write Tests For:
- ❌ Simple wrappers (ReadFile, WriteFile, Exists)
- ❌ Trivial getters/setters
- ❌ String concatenation
- ❌ Constructors without logic
- ❌ Defensive error handling (defer close, I/O failures)

### Remember:
- **Coverage is a side effect**, not the goal
- **45 meaningful tests** beats **60 tests with noise**
- **Security tests are non-negotiable** (validator must be 90%+)
- **Integration tests count** (they often test wrappers)
- **79% meaningful coverage** > **95% inflated coverage**
