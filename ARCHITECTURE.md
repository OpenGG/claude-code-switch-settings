# Architecture Documentation

## Overview

Claude Code Switcher (`ccs`) follows a clean architecture pattern with clear separation of concerns. The codebase is organized into focused layers:

- **Domain Layer**: Core business entities and errors
- **Service Layer**: Specialized services for specific concerns
- **Storage Layer**: Low-level file operations with security validations
- **Manager Layer**: Thin orchestrator coordinating services
- **CLI Layer**: Command-line interface adapters

## Directory Structure

```
internal/ccs/
├── domain/                 # Domain entities and errors
│   └── errors.go          # Domain error definitions
├── validator/             # Name validation service
│   └── validator.go       # Settings name validation logic
├── storage/               # Storage abstraction
│   └── storage.go         # Secure file operations
├── backup/                # Backup management
│   └── service.go         # Content-addressed backups
├── settings/              # Settings persistence
│   └── service.go         # Settings CRUD operations
└── manager.go             # Orchestrator (thin coordinator)
```

## Architecture Layers

### 1. Domain Layer (`internal/ccs/domain`)

**Purpose**: Define core business entities and domain errors.

**Responsibilities**:
- Define domain-specific errors
- Provide clear error semantics for business rules

**Example**:
```go
var (
    ErrSettingsNameEmpty    = errors.New("settings name cannot be empty")
    ErrSettingsNameNullByte = errors.New("settings name contains null byte")
    // ... other domain errors
)
```

**Dependencies**: None (pure domain logic)

### 2. Validator Service (`internal/ccs/validator`)

**Purpose**: Validate settings names for security and compatibility.

**Responsibilities**:
- Validate names against security rules (null bytes, path traversal)
- Check filesystem compatibility (invalid chars, reserved names)
- Normalize names (trim whitespace)

**Key Methods**:
- `ValidateName(name string) (bool, error)` - Validate a settings name
- `NormalizeName(name string) (string, error)` - Trim and validate

**Dependencies**: `domain` (for error types)

### 3. Storage Service (`internal/ccs/storage`)

**Purpose**: Provide low-level file operations with security validations.

**Responsibilities**:
- Symlink attack protection via `ValidatePathSafety()`
- Atomic file copies using temp files + rename
- Secure file permissions (0600) and directories (0700)
- Abstraction over `afero.Fs` filesystem

**Key Methods**:
- `CopyFile(src, dst string) error` - Atomic copy with security checks
- `ValidatePathSafety(path string) error` - Detect symlinks
- `ReadFile`, `WriteFile`, `Exists`, etc. - Secure file operations

**Security Features**:
- Uses `LstatIfPossible()` to detect symlinks before operations
- All files created with `0600` permissions (owner-only)
- All directories created with `0700` permissions (owner-only)
- Atomic rename prevents partial writes

**Dependencies**: `afero` (filesystem abstraction)

### 4. Backup Service (`internal/ccs/backup`)

**Purpose**: Manage content-addressed backups with SHA-256 hashing.

**Responsibilities**:
- Calculate SHA-256 hashes of files
- Create deduplicated backups (same content = same backup file)
- Update modification times for existing backups
- Prune old backups based on mtime

**Key Methods**:
- `CalculateHash(path string) (string, error)` - SHA-256 hash
- `BackupFile(path string) error` - Content-addressed backup
- `PruneBackups(olderThan time.Duration) (int, error)` - Delete old backups

**Content Addressing**:
- Backups stored as `<sha256-hash>.json`
- Empty files backed up as `empty.json` (with warning logged)
- Deduplication: identical content shares one backup file
- Mtime updated on each backup event for prune logic

**Dependencies**: `storage`, `slog` (logging)

### 5. Settings Service (`internal/ccs/settings`)

**Purpose**: Handle settings persistence and retrieval.

**Responsibilities**:
- Read/write active settings state
- List stored settings profiles
- Generate list entries with status annotations
- Provide path helpers for settings files

**Key Methods**:
- `GetActiveName() string` - Current active profile name
- `SetActiveName(name string) error` - Update active profile
- `ListStored() ([]string, error)` - All stored profile names
- `ListEntries(...) ([]ListEntry, error)` - Annotated list for display

**List Entry Annotations**:
- `*` prefix = active profile
- `!` prefix = missing profile (referenced but not found)
- `(active)` qualifier = currently active
- `(modified)` qualifier = active profile has local modifications

**Dependencies**: `storage`

### 6. Manager (Orchestrator) (`internal/ccs/manager.go`)

**Purpose**: Thin orchestrator that coordinates services to implement high-level operations.

**Responsibilities**:
- Initialize infrastructure (create directories)
- Orchestrate `Use` operation (validate → backup → copy → update state)
- Orchestrate `Save` operation (validate → backup → copy → update state)
- Delegate to services for specialized operations
- Expose public API with backward compatibility

**Key Design Principles**:
- **Thin**: Manager has NO business logic, only coordination
- **Delegation**: All work delegated to specialized services
- **Composition**: Services injected via constructor (dependency injection)
- **Backward Compatible**: Maintains same public API as old monolithic Manager

**Example Flow** (`Use` operation):
```go
func (m *Manager) Use(name string) error {
    // 1. Initialize infrastructure
    m.InitInfra()

    // 2. Validate name (delegates to validator)
    normalized := m.validator.NormalizeName(name)

    // 3. Check profile exists (delegates to settings)
    exists := m.settings.Exists(normalized)

    // 4. Backup current settings (delegates to backup)
    m.backup.BackupFile(activeSettingsPath)

    // 5. Copy profile to active location (delegates to storage)
    m.storage.CopyFile(storedPath, activeSettingsPath)

    // 6. Update active state (delegates to settings)
    m.settings.SetActiveName(normalized)
}
```

**Dependencies**: ALL services (validator, storage, backup, settings)

### 7. CLI Layer (`internal/cli`)

**Purpose**: Command-line interface adapters.

**Responsibilities**:
- Parse command-line arguments
- Present user-facing prompts and output
- Handle errors and format messages
- Call Manager methods for operations

**Dependencies**: `ccs.Manager`, `cobra`, `promptui`

## Dependency Graph

```
┌─────────────┐
│   CLI Layer │
└──────┬──────┘
       │
       v
┌─────────────────────────────────────┐
│          Manager (Orchestrator)      │
└──────┬─────┬─────┬─────┬───────────┘
       │     │     │     │
       v     v     v     v
    ┌───┐ ┌───┐ ┌───┐ ┌────┐
    │Val│ │Sto│ │Bak│ │Set │  Services
    └─┬─┘ └───┘ └─┬─┘ └─┬──┘
      │           │     │
      v           v     v
   ┌─────────────────────┐
   │    Domain Errors    │
   └─────────────────────┘
```

**Legend**:
- Val = Validator
- Sto = Storage
- Bak = Backup
- Set = Settings

## Design Patterns

### 1. Dependency Injection

Services are injected into Manager via constructor:

```go
func NewManager(fs afero.Fs, homeDir string, logger *slog.Logger) *Manager {
    storage := storage.New(fs)
    backup := backup.New(storage, backupDir, logger)
    settings := settings.New(storage, storeDir, activeState)
    validator := validator.New()

    return &Manager{
        storage:   storage,
        backup:    backup,
        settings:  settings,
        validator: validator,
    }
}
```

**Benefits**:
- Easy to test (inject mocks)
- Clear dependencies
- Loose coupling

### 2. Interface Segregation

Each service has a focused interface with minimal methods:

- **Validator**: `ValidateName()`, `NormalizeName()`
- **Storage**: File operations only
- **Backup**: Backup operations only
- **Settings**: Settings persistence only

**Benefits**:
- Single Responsibility Principle
- Easy to understand and maintain
- Testable in isolation

### 3. Composition Over Inheritance

Manager composes services rather than inheriting behavior:

```go
type Manager struct {
    validator *validator.Validator
    storage   *storage.Storage
    backup    *backup.Service
    settings  *settings.Service
}
```

**Benefits**:
- Flexible composition
- No diamond problem
- Clear object boundaries

## Testing Strategy

The codebase prioritizes **test quality over coverage numbers**. See [TESTING.md](TESTING.md) for comprehensive testing guidelines.

### Test Architecture

Each layer is tested according to its complexity and risk:

**Security-Critical (Comprehensive)**:
- **Validator**: Path traversal, null bytes, control chars, reserved names, Unicode attacks

**Complex Business Logic (Thorough)**:
- **Backup**: SHA-256 hashing, content-addressed deduplication, time-based pruning
- **Settings**: State machine (5 states: active/modified/missing/unsaved/inactive)

**Infrastructure (Integration-Tested)**:
- **Storage**: Atomic operations, symlink protection, secure permissions
- **Manager**: End-to-end workflows (Use → Save → List cycles)

**Not Tested**:
- Simple wrappers (ReadFile, WriteFile, Exists) - covered by integration tests
- Trivial getters/setters - no logic to verify

### Test Fixtures

All tests use `afero.MemMapFs()` for fast, isolated, in-memory filesystem testing:

```go
func newTestManager(t *testing.T) *Manager {
    t.Helper()
    fs := afero.NewMemMapFs()                     // In-memory filesystem
    mgr := NewManager(fs, "/home/test", nil)      // nil logger = discard
    mgr.InitInfra()                               // Create test directories
    return mgr
}
```

### Coverage Philosophy

Target: **~80% meaningful coverage**
- Security validation: >90% required
- Complex logic: 80%+ expected
- Simple wrappers: 0% acceptable (integration-tested)
- Defensive error handling: 0% acceptable (untestable without mocking)

Current coverage reflects meaningful tests, not inflated numbers. See [TESTING.md](TESTING.md) for detailed rationale.

## Security Architecture

### Defense in Depth

Multiple layers of security validation:

1. **Validator**: Checks for malicious names (null bytes, path traversal)
2. **Storage**: Validates symlinks before operations
3. **Manager**: Orchestrates validation before any file operations

### File Permissions

- **Directories**: `0700` (drwx------)
- **Files**: `0600` (-rw-------)
- **Rationale**: Prevent unauthorized access on multi-user systems

### Atomic Operations

All file replacements use atomic rename:

```go
func (s *Storage) CopyFile(src, dst string) error {
    // 1. Write to temp file
    tmp := dst + ".tmp"
    writeToFile(tmp, data)

    // 2. Atomic rename (overwrites dst atomically)
    fs.Rename(tmp, dst)  // No window where dst is missing!
}
```

**Benefit**: No data loss if process crashes mid-operation

### Symlink Protection

```go
func (s *Storage) ValidatePathSafety(path string) error {
    info := fs.Lstat(path)  // Don't follow symlinks!
    if info.Mode() & os.ModeSymlink != 0 {
        return error("refusing to operate on symlink")
    }
}
```

**Benefit**: Prevents symlink attacks (e.g., `ln -s /etc/passwd ~/.claude/settings.json`)

## Migration from Old Architecture

### Before (Monolithic Manager)

```go
type Manager struct {
    fs      afero.Fs
    homeDir string
    now     func() time.Time
    logger  *slog.Logger

    // All logic in one struct:
    // - Validation
    // - File operations
    // - Backup logic
    // - Settings persistence
}
```

**Problems**:
- God object anti-pattern
- Hard to test individual concerns
- Mixed responsibilities
- 400+ lines of code

### After (Clean Architecture)

```go
type Manager struct {
    homeDir   string
    validator *validator.Validator    // 60 lines
    storage   *storage.Storage        // 100 lines
    backup    *backup.Service         // 150 lines
    settings  *settings.Service       // 100 lines
}
```

**Benefits**:
- Single Responsibility Principle
- Easy to test each service
- Clear boundaries
- Manager is ~100 lines (just orchestration)

### Backward Compatibility

Public API unchanged:

```go
// Same method signatures
mgr.Use(name string) error
mgr.Save(targetName string) error
mgr.ValidateSettingsName(name string) (bool, error)
mgr.CalculateHash(path string) (string, error)
// ... etc
```

**Benefit**: No breaking changes for CLI layer or existing tests

## Future Enhancements

### Potential Improvements

1. **Add unit tests for services**: While services are tested through Manager, dedicated unit tests would improve coverage

2. **Extract interfaces**: Define `Validator`, `Storage`, `Backup`, `Settings` interfaces for better testability

3. **Version settings schema**: Add version field to profile files for future compatibility

4. **Encryption support**: Add optional encryption for sensitive settings

5. **Remote sync**: Support syncing profiles to cloud storage

### Adding New Features

To add a new feature:

1. **Identify the concern**: Which service owns this responsibility?
2. **Add to service**: Implement in the appropriate service (or create new one)
3. **Update Manager**: Add orchestration method if needed
4. **Update CLI**: Add new command or option
5. **Add tests**: Test service and integration through Manager

**Example**: Adding settings diffing

```go
// 1. Add to settings service
func (s *Settings) Diff(name1, name2 string) ([]Change, error)

// 2. Add to Manager
func (m *Manager) DiffSettings(name1, name2 string) ([]Change, error) {
    return m.settings.Diff(name1, name2)
}

// 3. Add CLI command
func newDiffCommand(mgr *ccs.Manager) *cobra.Command {
    // Use mgr.DiffSettings()
}
```

## Conclusion

The clean architecture provides:

- ✅ **Testability**: Each layer testable in isolation
- ✅ **Maintainability**: Clear boundaries and responsibilities
- ✅ **Security**: Defense-in-depth validation
- ✅ **Flexibility**: Easy to add/modify services
- ✅ **Backward Compatibility**: Same public API

Total lines of code reduced from ~800 to ~500 with better organization.
