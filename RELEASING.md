# Releasing Revoco

This guide covers the setup required for automated multi-channel distribution of Revoco via GitHub Actions and GoReleaser.

## Required GitHub Secrets

You must configure these secrets in your repository settings under **Settings > Secrets and variables > Actions**.

| Secret Name | Purpose | Where to Obtain |
|-------------|---------|-----------------|
| `AUR_SSH_PRIVATE_KEY` | Publish to Arch User Repository (AUR) | Generate an Ed25519 SSH key pair. Add the public key to your AUR account settings. |
| `CHOCOLATEY_API_KEY` | Publish to Chocolatey | Create an account at [chocolatey.org](https://www.chocolatey.org/), then go to **My Account > API Keys**. |
| `WINGET_TOKEN` | Submit to Windows Package Manager | Create a GitHub Personal Access Token (classic) with `public_repo` scope. |
| `GH_PAT` | Push to Homebrew and Scoop repos | Create a GitHub Personal Access Token (classic) with `repo` scope. This is required because `GITHUB_TOKEN` cannot trigger subsequent workflows or push to other repositories. |

## External Accounts & Repositories

Before the first release, ensure you have set up these accounts and repositories.

### GitHub Repositories
Create these public repositories under your account. They can be initialized with a README or kept empty.
- `homebrew-revoco` — Your personal Homebrew tap.
- `scoop-revoco` — Your personal Scoop bucket.

### Package Registries
- **AUR (Arch Linux)**: Create an account at [aur.archlinux.org](https://aur.archlinux.org/). You will need to submit the initial PKGBUILD or let GoReleaser attempt the first push (requires the package name to be available).
- **Chocolatey**: Create an account at [chocolatey.org](https://www.chocolatey.org/).

## First Release Checklist

Follow these steps for a smooth first-time distribution:

1. **Verify Configs**: Ensure `.goreleaser.yaml` and `.github/workflows/release.yml` are present in the repository.
2. **Create Repos**: Create `homebrew-revoco` and `scoop-revoco` on GitHub.
3. **Configure Secrets**: Add all secrets listed above to your main `revoco` repository.
4. **Local Test**: Run `goreleaser release --snapshot --clean` to verify the build process (this won't publish).
5. **Tag & Push**:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
6. **Monitor Actions**: Check the **Actions** tab in GitHub to ensure the release workflow completes successfully.
7. **Verify Channels**:
   - Check [GitHub Releases](https://github.com/fulgidus/revoco/releases) for binaries and changelog.
   - Run `brew tap <your-user>/revoco && brew install revoco`.
   - Run `scoop bucket add revoco https://github.com/<your-user>/scoop-revoco && scoop install revoco`.
   - Check AUR and Chocolatey pending queues.
   - Check Winget PR status at [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs/pulls).
