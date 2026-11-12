# Release Process Example

This document provides a step-by-step example of creating a release for the Reddit Cluster Map project.

## Prerequisites

- Maintainer access to the repository
- All tests passing on main branch
- Changes merged and ready for release

## Example: Creating Release v1.0.0

### Step 1: Prepare Your Local Environment

```bash
# Ensure you're on main branch and up to date
git checkout main
git pull origin main

# Verify clean working directory
git status
```

### Step 2: Run Tests

```bash
# Backend tests
cd backend
make test
make test-integration-docker
make lint

# Frontend tests
cd ../frontend
npm ci
npm run build
npm run lint

cd ..
```

### Step 3: Update VERSION File

```bash
# Update the version number
echo "1.0.0" > VERSION

# Verify the change
cat VERSION
```

### Step 4: Update CHANGELOG.md

Edit `CHANGELOG.md` and move items from `[Unreleased]` to a new version section:

```markdown
## [Unreleased]

## [1.0.0] - 2024-01-15

### Added
- 3D force-directed graph visualization
- Automated community detection using Louvain algorithm
- Real-time crawler with Redis job queue
- PostgreSQL database with optimized schema
- Docker Compose development environment
- CI/CD pipeline with automated releases

### Changed
- Migrated from SQLite to PostgreSQL
- Improved graph precalculation performance by 300%
- Updated frontend to React 19

### Fixed
- Fixed crawler rate limiting issues
- Corrected graph layout calculations
- Resolved memory leaks in graph rendering

[Unreleased]: https://github.com/subculture-collective/clustr/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/subculture-collective/clustr/releases/tag/v1.0.0
```

### Step 5: Commit Changes

```bash
# Stage the changes
git add VERSION CHANGELOG.md

# Commit with conventional commit message
git commit -m "chore: release v1.0.0"

# Verify the commit
git --no-pager log -1
```

### Step 6: Create and Push Git Tag

```bash
# Create an annotated tag
git tag -a v1.0.0 -m "Release v1.0.0"

# Verify the tag
git --no-pager tag -l -n1 v1.0.0

# Push commit and tag to GitHub
git push origin main
git push origin v1.0.0
```

### Step 7: Automated Release Process

Once the tag is pushed, GitHub Actions automatically:

1. **Release Workflow** (`.github/workflows/release.yml`):
   - Triggers on the `v1.0.0` tag
   - Generates changelog from commit messages
   - Creates a GitHub Release with release notes
   - Marks as production release (no `-` in version)

2. **Publish Workflow** (`.github/workflows/publish.yml`):
   - Triggers on the `v1.0.0` tag
   - Builds Docker images for all services:
     - `ghcr.io/subculture-collective/clustr-server`
     - `ghcr.io/subculture-collective/clustr-crawler`
     - `ghcr.io/subculture-collective/clustr-precalculate`
     - `ghcr.io/subculture-collective/clustr-frontend`
   - Tags images with:
     - `v1.0.0` (version tag)
     - `v1.0` (minor version)
     - `v1` (major version)
     - `latest` (latest stable)
     - `sha-abc123` (commit SHA)

### Step 8: Verify the Release

```bash
# Check GitHub Releases page
open https://github.com/subculture-collective/clustr/releases

# Pull and verify Docker images
docker pull ghcr.io/subculture-collective/clustr-server:v1.0.0
docker pull ghcr.io/subculture-collective/clustr-crawler:v1.0.0
docker pull ghcr.io/subculture-collective/clustr-precalculate:v1.0.0
docker pull ghcr.io/subculture-collective/clustr-frontend:v1.0.0

# Verify image tags
docker images | grep reddit-cluster-map
```

### Step 9: Test the Release

```bash
# Create a test directory
mkdir -p /tmp/test-release
cd /tmp/test-release

# Download docker-compose.yml (update image tags to v1.0.0)
# Test the release
docker compose up -d
docker compose ps
docker compose logs

# Clean up
docker compose down
```

### Step 10: Announce the Release

- Update project documentation if needed
- Announce in project discussions or relevant channels
- Update any external references to the project
- Monitor for issues or user feedback

## Example: Creating Pre-release v1.1.0-beta.1

For pre-releases, follow the same process but use a pre-release version format:

```bash
# Update version
echo "1.1.0-beta.1" > VERSION

# Update CHANGELOG.md
## [Unreleased]

## [1.1.0-beta.1] - 2024-02-01

### Added
- Experimental search feature (beta)
- New graph filtering options (testing)

# Commit and tag
git add VERSION CHANGELOG.md
git commit -m "chore: release v1.1.0-beta.1"
git tag -a v1.1.0-beta.1 -m "Release v1.1.0-beta.1 (Beta)"
git push origin main
git push origin v1.1.0-beta.1
```

GitHub Actions will automatically mark this as a pre-release because the tag contains a `-` character.

## Example: Creating Hotfix Release v1.0.1

For critical bug fixes:

```bash
# Create hotfix branch from release tag
git checkout -b hotfix/1.0.1 v1.0.0

# Make the fix
# ... edit files ...

# Test the fix
cd backend && make test

# Update version and changelog
echo "1.0.1" > VERSION
# Edit CHANGELOG.md to add hotfix section

# Commit all changes together
git add VERSION CHANGELOG.md
git commit -m "fix: critical bug in crawler rate limiter

- Fixed race condition in rate limiter
- Updated version to 1.0.1
- Updated CHANGELOG.md"

# Merge to main and tag
git checkout main
git merge hotfix/1.0.1
git tag -a v1.0.1 -m "Release v1.0.1 - Hotfix"
git push origin main
git push origin v1.0.1

# Clean up branch
git branch -d hotfix/1.0.1
```

## Troubleshooting

### Release Workflow Failed

```bash
# Check workflow status
open https://github.com/subculture-collective/clustr/actions

# Common issues:
# - Permissions: Ensure workflow has write permissions
# - Tag format: Must be v*.*.* (semantic versioning)
# - Previous tag: Ensure previous release tag exists
```

### Docker Image Build Failed

```bash
# Test Docker build locally
cd backend
docker build -t test:local \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -f Dockerfile .
```

### Wrong Tag Pushed

```bash
# Delete tag locally and remotely
git tag -d v1.0.0
git push origin :refs/tags/v1.0.0

# Delete release on GitHub (manually via web interface)
# Re-create tag with correct version
```

## Best Practices

1. **Test Thoroughly**: Always run full test suite before releasing
2. **Clear Changelog**: Write clear, user-friendly changelog entries
3. **Version Bumping**: Follow semantic versioning strictly
4. **Pre-releases**: Use pre-releases for testing major changes
5. **Communication**: Announce breaking changes in advance
6. **Hotfixes**: Release hotfixes quickly for critical bugs
7. **Documentation**: Update docs before or with the release
8. **Monitoring**: Monitor logs and metrics after release

## See Also

- [CHANGELOG.md](../CHANGELOG.md) - Project changelog
- [VERSION](../VERSION) - Current version number
- [CONTRIBUTING.md](../CONTRIBUTING.md#release-process) - Full release process documentation
- [CI-CD.md](CI-CD.md) - CI/CD pipeline documentation
