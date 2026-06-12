# SatPulse release process

This release process should be performed in a VSCode devcontainer:
1. Open the repository in VSCode
2. For minor releases, consider rebuilding without cache to get latest tools:
   - Command Palette: "Dev Containers: Rebuild Container Without Cache"
3. Reopen in Container when prompted (or use Command Palette: "Dev Containers: Reopen in Container")
4. Open a terminal within VSCode to run the commands below

## Overview

- **Prereleases**: Built automatically from any commit without a matching version tag
- **Final releases**: Built when commit has tag matching `v$(VERSION)` and working tree is clean
- **Version source**: `VERSION` file in repository root

## Version formats

| Type | Debian Package | RPM Package | Binary Version |
|------|---------------|-------------|----------------|
| Prerelease | `0.1~git20250901.abc123-1` | `0.1~20250901gitabc123-1` | `0.1-pre.20250901.abc123` |
| Final | `0.1-1` | `0.1-1` | `0.1` |

## Development workflow

### Building prereleases

During normal development, all builds are prereleases:

```bash
# Build packages
make pkg

# Create draft GitHub release with prerelease packages
make release
```

### Testing with Ansible

Deploy and test on real hardware:

```bash
cd systest
./t install  # Deploy packages to test machines
./t check    # Run validation tests
```

## Release process

### 1. Prepare for release

Ensure all changes are committed and tests pass:

```bash
git status  # Should be clean
make test   # Run unit tests
```

### 2. Create release tag

Create local annotated tag (reversible):

```bash
make tag
# Creates tag v0.1 (or whatever VERSION contains)
```

### 3. Build final release packages

With the tag in place, builds produce final versions:

```bash
make pkg
```

Verify version strings:

```bash
out/amd64/satpulsetool --version
# Should show: satpulse version 0.1, build date ...
```

### 4. Hardware testing

Deploy final packages to test hardware:

```bash
cd systest
./t install
./t check
```

If issues are found, you can fix them and move the tag to a different commit:

```bash
# Fix issues, commit...
make tag    # Moves the tag to current commit
```

To abandon the release process:

```bash
make untag  # Remove the tag entirely
```

### 5. Create draft GitHub release

```bash
make release
# Creates draft release on GitHub with final packages
```

On GitHub, edit the description.

### 6. Finalize release (point of no return)

Once testing is complete and you're ready to publish:

```bash
version_tag="v$(cat VERSION)"
# Push the tag to GitHub
git push origin ${version_tag}
# Publish the draft release on GitHub
gh release edit ${version_tag} --draft=false
```
Note: After testing on a real release, we can make this a `finalize-release` Makefile target.

### 7. Post-release version management

After publishing a release, update VERSION for next development:

#### For minor release (e.g., after releasing 0.1)

```bash
current_version=$(cat VERSION)
# Create maintenance branch from release tag
git checkout -b v${current_version} v${current_version}
# Set up for patch releases on branch
echo -n "${current_version}.1" >VERSION
git add docs/_includes/VERSION
git commit -m "Prepare for $(cat VERSION) patch release"
# Push the maintenance branch (use refs/heads/ to disambiguate from tag)
git push -u origin refs/heads/v${current_version}
# Switch back to master and bump minor version
git checkout master
next_version=$(awk -F. '{print $1"."$2+1}' VERSION)
echo -n "${next_version}" >VERSION
git add docs/_includes/VERSION
git commit -m "Bump version to ${next_version} for next development cycle"
git push origin master
```

Also set `man_prerelease_notice: false` in `docs/_config.yml` to remove
the pre-release banner from the man pages on the website.
Turn it back on later if and when the man pages on master diverge
significantly from the last release.

#### For patch release (e.g., 0.1.1)

After publishing a patch release:

```bash
# Prepare for next patch
next_patch=$(awk -F. '{print $1"."$2"."$3+1}' VERSION)
echo "${next_patch}" > VERSION
git add VERSION
git commit -m "Prepare for ${next_patch} patch release"
# Push
git push
```

## Quick reference

### Essential commands

| Command | Description |
|---------|-------------|
| `make tag` | Create local release tag |
| `make untag` | Remove local release tag |
| `make pkg` | Build packages |
| `make release` | Create GitHub draft release |
| `git push origin v0.1` | Push tag to GitHub (finalize) |

### Version checks

```bash
# Check current version
cat VERSION

# Check if on a release tag
git describe --tags --exact-match

# Check package versions
ls -la out/*.deb
ls -la out/*.rpm
```
