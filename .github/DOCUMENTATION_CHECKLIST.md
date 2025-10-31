# Documentation Maintenance Checklist

This checklist ensures documentation stays accurate, complete, and free of duplication.

## Before Every Release

### README.md
- [ ] Installation instructions are current
- [ ] All commands have examples
- [ ] New features documented in Usage section
- [ ] Security section reflects current implementation
- [ ] No references to removed/renamed features

### README.zh-cn.md (Chinese Translation)
- [ ] Synchronized with README.md changes
- [ ] All technical terms accurately translated
- [ ] Links point to English version for reference
- [ ] No outdated information (check: SHA-256, not MD5)

### CHANGELOG.md
- [ ] Move `[Unreleased]` changes to new version section
- [ ] Add version number and date
- [ ] Breaking changes clearly marked
- [ ] Security fixes highlighted
- [ ] Follows [Keep a Changelog](https://keepachangelog.com/) format

### ARCHITECTURE.md
- [ ] Directory structure reflects current codebase
- [ ] All services documented
- [ ] Dependency graph is accurate
- [ ] Design patterns section updated if patterns changed
- [ ] No references to deleted files/packages

### CLAUDE.md
- [ ] Build commands work
- [ ] Test commands work
- [ ] Coverage threshold is current
- [ ] Directory structure matches ARCHITECTURE.md
- [ ] Release process steps are current

---

## On Architecture Changes

When refactoring or adding new services:

1. **Update ARCHITECTURE.md** (primary source)
   - Add new service/layer description
   - Update dependency graph
   - Add to "Architecture Layers" section

2. **Update CLAUDE.md** (brief reference)
   - Update directory structure diagram
   - Add one-line description to directory structure

3. **Update CHANGELOG.md**
   - Document in "Architecture" or "Changed" section
   - Note any breaking changes

4. **Do NOT update README.md** unless user-facing behavior changes

---

## On Feature Changes

When adding commands or changing behavior:

1. **Update README.md** (primary source)
   - Add to Usage section with examples
   - Update relevant sections (Installation, Configuration, etc.)

2. **Update CHANGELOG.md**
   - Add to "Added" or "Changed" section
   - Include examples if behavior changed

3. **Update CLAUDE.md** only if AI assistant needs to know
   - Usually no changes needed unless testing or build changes

4. **Do NOT update ARCHITECTURE.md** unless internal structure changed

---

## Documentation Quality Standards

Every document must pass these checks:

### Correctness ✓
- [ ] No references to deleted files/functions
- [ ] No outdated algorithm names (e.g., MD5 when using SHA-256)
- [ ] All code references include correct line numbers or use fuzzy references
- [ ] All commands actually work when run

### Completeness ✓
- [ ] Covers all major features/components
- [ ] No critical information missing
- [ ] Appropriate depth for target audience

### Clarity ✓
- [ ] Clear section headings
- [ ] Logical organization
- [ ] Code examples where helpful
- [ ] Technical terms explained

### Conciseness ✓
- [ ] No redundant information
- [ ] No duplication across documents (use references instead)
- [ ] As short as possible while remaining complete

### Currency ✓
- [ ] Reflects current state of codebase
- [ ] No stale information
- [ ] Update date noted where appropriate

---

## Single Source of Truth (SSOT) Rules

Each type of information has ONE authoritative location:

| Information | SSOT | May Reference From |
|-------------|------|-------------------|
| **How to install** | README.md | - |
| **How to use (commands)** | README.md | CLAUDE.md (brief) |
| **How it works (architecture)** | ARCHITECTURE.md | CLAUDE.md (brief), README.md (very brief) |
| **What changed** | CHANGELOG.md | - |
| **Security model** | README.md (Security section) | ARCHITECTURE.md (implementation details) |
| **Testing strategy** | ARCHITECTURE.md | CLAUDE.md (conventions only) |
| **Build/test commands** | CLAUDE.md | README.md (Contributing section) |
| **Release process** | CLAUDE.md | README.md (note only) |

**Rule:** When information is needed in multiple places, create ONE detailed explanation in the SSOT, then REFERENCE it from other documents. Do NOT duplicate.

**Example:**
```markdown
# README.md
## How Backups Work
[Detailed 3-paragraph explanation]

# CLAUDE.md
- **Backup system**: SHA-256 content-addressed (see README.md "How Backups Work")
```

---

## Anti-Patterns to Avoid

### ❌ Duplication
**Bad:**
- Same commands in README.md, CLAUDE.md, and AGENTS.md
- Same architecture explanation in multiple files

**Good:**
- Commands in CLAUDE.md (detailed) + README.md (basic)
- Architecture in ARCHITECTURE.md (detailed) + CLAUDE.md (brief reference)

### ❌ Temporary Docs Left in Repo
**Bad:**
- `FIXES_IMPLEMENTED.md` still present after implementation
- `ARCHITECTURE_REVIEW.md` still present after addressing all issues

**Good:**
- Delete temporary docs when their purpose is served
- Move relevant information to permanent docs first

### ❌ Unmaintained Translations
**Bad:**
- `README.zh-cn.md` says "MD5" when English version says "SHA-256"

**Good:**
- Either commit to maintaining translations in sync
- OR delete them to avoid misleading users

### ❌ Scope Creep
**Bad:**
- README.md with 500 lines of architectural details
- ARCHITECTURE.md with installation instructions

**Good:**
- README.md focuses on usage
- ARCHITECTURE.md focuses on internal design
- Clear boundaries between document purposes

---

## Automated Checks

Run these before every release:

```bash
# Verify all markdown links work
# (Requires: npm install -g markdown-link-check)
find . -name '*.md' -exec markdown-link-check {} \;

# Check for references to deleted files
! grep -r "paths\.go" *.md
! grep -r "manager_old\.go" *.md

# Verify key locations are documented
grep -q "~/.claude/settings.json" README.md
grep -q "SHA-256" README.md

# Ensure CHANGELOG is updated
! grep -q "## \[Unreleased\]" CHANGELOG.md || echo "WARNING: Update CHANGELOG before release"

# Count markdown files (should be exactly 5)
[ $(ls -1 *.md | wc -l) -eq 5 ] || echo "ERROR: Should have exactly 5 .md files"

# Verify Chinese translation is in sync (check key terms)
grep -q "SHA-256" README.zh-cn.md || echo "WARNING: Chinese README may be outdated"
```

---

## Documentation Health Score

Score each document 0-5 on each metric:

| Metric | Weight |
|--------|--------|
| Correctness | 2x |
| Completeness | 2x |
| Clarity | 1x |
| Conciseness | 1x |
| Currency | 1x |

**Formula:** `(Correctness×2 + Completeness×2 + Clarity + Conciseness + Currency) / 7`

**Thresholds:**
- **4.5-5.0**: Excellent ✅
- **3.5-4.4**: Good 👍
- **2.5-3.4**: Needs improvement ⚠️
- **< 2.5**: Critical - fix or delete ❌

---

## Current Documentation Structure

```
.
├── README.md              # User documentation (install, use, security)
├── README.zh-cn.md        # Chinese translation of README.md
├── ARCHITECTURE.md        # Developer documentation (how it works)
├── CHANGELOG.md          # Release history (what changed)
└── CLAUDE.md             # AI assistant guidance (conventions, commands)
```

**Total:** 5 files

**Translations:** README.zh-cn.md must be kept in sync with README.md

**If adding a new .md file, ask:**
1. What audience does this serve?
2. What unique purpose does it have?
3. Does it duplicate existing docs?
4. Is it permanent or temporary?
5. Who will maintain it?

If you can't answer all 5 questions clearly, don't create the file.
