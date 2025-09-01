# Semantic versioning implementation plan for SatPulse

## 1. Current situation

### Git-based versioning
- **Debian packages:** `0.0~gitYYYYMMDD.HASH[-1]` (e.g., `0.0~git20250831.abc123-1`)
- **RPM packages:** `0^YYYYMMDDgitHASH-1` (e.g., `0^20250831gitabc123-1`)
- **Release symlinks:** Simple date format `YYYYMMDD`
- **Version embedding:** Git hash and build date passed via ldflags to Go binaries
- **Release process:** Creates draft GitHub releases with date-based tags

### Key observations
- Prefixes `0.0~` (Debian) and `0^` (RPM) ensure any future semantic version will sort higher
- No semantic versioning structure
- No distinction between prereleases and final releases

## 2. New versioning plan

### Version source
- `VERSION` file containing next release version (e.g., `0.1`)
- Git annotated tags (`v0.1`) mark final releases
- Absence of matching tag indicates prerelease

### Version formats

| Type | Debian | RPM | Symlink | Go Binary |
|------|--------|-----|---------|-----------|
| **Prerelease** | `0.1~gitYYYYMMDD.HASH-1` | `0.1~YYYYMMDDgitHASH-1` | `0.1-pre-YYYYMMDD` | `0.1-pre.YYYYMMDD.HASH[.dirty]` |
| **Final release** | `0.1-1` | `0.1-1` | `0.1` | `0.1` |

### Detection logic
- Check if current commit has tag matching `v$(VERSION)`
- If yes → final release
- If no → prerelease

## 3. Release process

### A. Development phase
1. Work on features/fixes
2. Build prereleases as needed: `make release`
   - Creates packages with prerelease versions
   - Creates draft GitHub release
   - Symlinks have `-pre-YYYYMMDD` suffix

### B. Final release preparation (reversible)
1. **Tag creation:** `make tag`
   - Creates annotated tag `v$(VERSION)` locally only
   - Ensures working tree is clean
   - Tag message: "Release v$(VERSION)"
   - **Note:** Tag remains local, can be deleted if issues found

2. **Build and test final packages:** `make release`
   - Detects local tag match → builds without prerelease suffixes
   - Creates draft GitHub release
   - Allows thorough testing of final artifacts
   - **Can rebuild:** Delete local tag, fix issues, retag, rebuild

3. **Hardware testing:** Use Ansible for real hardware validation
   - Deploy packages: `cd systest && ./t install` (wrapper for ansible-playbook)
   - Run validation: `./t check` or other test playbooks
   - Tests actual GPS hardware and PTP clock synchronisation
   - Essential step as CI/CD lacks specialised hardware
   - If issues found: delete tag, fix, rebuild

### C. Point of no return
4. **Finalize:** `make finalize-release`
   - **This is the only irreversible step**
   - Pushes annotated tag to remote
   - Publishes draft release on GitHub
   - Branch handling:
     - For initial x.y release (e.g., 0.1): creates `v0.x` branch
     - On the new branch: sets VERSION to `0.1.1`
     - On master: bumps VERSION (0.1 → 0.2)
   - For patch releases (on existing branch):
     - No new branch created
     - Bumps patch version (0.1.1 → 0.1.2)
   - Commits and pushes VERSION changes

## 4. Package version comparisons

### Debian versioning rules
- `~` sorts before anything (including empty)
- Current: `0.0~git20250831.abc123-1`
- Prerelease: `0.1~git20250901.def456-1` (higher: 0.1 > 0.0)
- Final: `0.1-1` (higher: no ~ > ~)
- Next prerelease: `0.2~git20250902.ghi789-1` (higher: 0.2 > 0.1)

### RPM versioning rules
- `^` sorts before numbers
- Current: `0^20250831gitabc123-1`
- Prerelease: `0.1~20250901gitdef456-1` (higher: 0.1 > 0^)
- Final: `0.1-1` (higher: no ~ > ~)
- Next: `0.2~20250902gitghi789-1` (higher: 0.2 > 0.1)

**Upgrade path guaranteed:** All new versions will sort higher than existing packages.

## 5. Implementation changes

### New files
- `VERSION` - Contains semantic version (initially `0.1`)

### Makefile changes
- Read `VERSION` file
- Detect tag match for final vs prerelease
- Adjust version strings accordingly
- Add `make tag` target
- Add `make finalize-release` target
- Update symlink patterns

### bsd-build.sh changes
- Read `VERSION` file
- Apply same versioning logic for Go binary version string

### systest/install.yml changes
- Update package patterns to match new versioning:
  - Debian: `satpulse_[0-9]+\.[0-9]+.*_{{ arch }}.deb` (catches both `0.1-1` and `0.1~git...`)
  - RPM: `satpulse-[0-9]+\.[0-9]+.*\.{{ arch }}.rpm`
- Sorting strategy:
  - **Use modification time**: Sort by `mtime` instead of filename
  - Latest built package = most recently modified
  - Works for both prereleases and final releases
  - Change: `sort(attribute='path')` → `sort(attribute='mtime')`

## 6. Transition timeline

1. **Phase 1:** Add VERSION file, update build logic
2. **Phase 2:** Test with prereleases (0.1~...)
3. **Phase 3:** Hardware testing with Ansible playbooks
4. **Phase 4:** First final release (0.1)
5. **Phase 5:** Establish 0.x branch for patches
6. **Phase 6:** Continue with 0.2 development

## 7. Example version progression

```
# Initial state (master branch)
VERSION: 0.1
Packages: 0.1~git20250901.abc123-1

# After tagging v0.1 (master branch)
VERSION: 0.1 (with tag v0.1)
Packages: 0.1-1

# After finalize-release for v0.1
master branch: VERSION: 0.2
v0.1 branch: VERSION: 0.1.1

# Development continues (master)
VERSION: 0.2
Packages: 0.2~git20250902.def456-1

# Patch release (v0.1 branch)
VERSION: 0.1.1 (with tag v0.1.1)
Packages: 0.1.1-1

# After finalize-release for v0.1.1 (v0.1 branch)
VERSION: 0.1.2
Packages: 0.1.2~git20250903.ghi789-1
```

## 8. Benefits

- **Backward compatible:** All new versions sort higher than existing packages
- **Clear versioning:** Semantic versions for better user understanding
- **Prerelease support:** Continued ability to test before final releases
- **Automation friendly:** Tag-based detection simplifies CI/CD
- **Standard compliance:** Follows Debian/RPM versioning conventions
- **Safe testing:** Everything reversible until `finalize-release` - can rebuild final packages multiple times
- **Draft releases:** GitHub releases stay draft until explicitly published
- **Hardware validation:** Ansible integration ensures real hardware testing before final release