# Desktop GUI issues

## macos-github-build: Build a macOS .dmg in GitHub Actions

The desktop GUI has no macOS distribution -- users clone the repo and build it themselves. Provide a downloadable `.dmg`, built in GitHub Actions on a macOS runner rather than on a developer's Mac. macOS cannot be containerised, so a clean, ephemeral, versioned CI runner (with pinned toolchain versions) is the practical way to get a reproducible build; an accreted local machine is not. This deliberately trades GitHub dependency for reproducibility, which is the priority here.

The workflow runs `wails build`, packages the result into a `.dmg`, and publishes it as a release asset.

Signing is an optional, slot-in stage. With no Apple Developer ID, produce an unsigned `.dmg` (quarantined by Gatekeeper, so right-click-to-open). When the signing secrets are present -- a Developer ID Application certificate plus notarization credentials -- `codesign` with the hardened runtime, submit with `notarytool`, and `staple`. So the pipeline works now and gains notarization once we have a Developer ID. A Homebrew cask could later point at the published `.dmg`, but a cask only installs cleanly once the artifact is notarized, so that follows signing.

Still to decide: whether the build is a universal (arm64 + amd64) binary or arm64-only, and the workflow trigger.

## windows-github-build: Build a Windows package in GitHub Actions

The desktop GUI has no Windows distribution -- users clone the repo and build it themselves, after a painful manual install of the build dependencies. Provide a downloadable build, produced in GitHub Actions on a `windows-latest` runner rather than on a developer's machine. As with the macOS build, Windows cannot be containerised on a non-Windows host, so a clean, ephemeral, versioned CI runner (with pinned toolchain versions) is the practical way to get a reproducible build; it also removes the manual dependency setup, since the runner already provides the MSVC toolchain, WebView2, Go, and Node. Reproducibility is the priority, deliberately traded against GitHub dependency.

The first step is an NSIS installer (`wails build -nsis`), published as a release asset. Unsigned, it still installs after a SmartScreen "unknown publisher" warning the user can click through -- unlike an unsigned MSIX, which Windows refuses to install at all -- so NSIS degrades gracefully without a code-signing certificate and is the right starting point. WebView2 is present on Windows 11 and on the runner.

We do not pursue an Authenticode code-signing certificate at all. It is a recurring cost that, since 2023, requires a hardware or cloud HSM key, and the Microsoft Store (the second step below) provides attestation without one. So the direct-download installer stays unsigned -- the SmartScreen warning is accepted -- and the warning-free experience comes from the Store rather than from signing the `.exe`.

The second step is the Microsoft Store. The draw is attestation: a Store app is signed and vouched for by Microsoft, sidestepping SmartScreen without us buying and maintaining our own certificate (and an individual Store developer account is a one-time fee of around $19). This targets an MSIX-packaged Win32 (full-trust desktop) app -- the existing Wails/WebView2 app wrapped in MSIX with the `runFullTrust` capability, not a UWP app -- so it is repackaging, not a rewrite. The Store distributes MSIX packages, a format that NSIS does not produce and that Wails does not build natively, so a prerequisite engineering step is being able to build an MSIX package -- the Windows SDK / `MakeAppx`, with our own package identity -- on top of the build, before any Store submission. A winget or scoop manifest could also point at the published installer, analogous to a Homebrew cask, but that is out of scope here.

Still to decide: the workflow trigger.

