# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Incremental Precalculation**: Graph precalculation now only processes entities that have changed since the last run
  - Reduces precalc time from 10+ minutes to <2 minutes for <5% data changes
  - Maintains stable node IDs across rebuilds
  - No service disruption for connected clients
  - Automatic mode selection based on change percentage (threshold: 20%)
  - Manual full rebuild via `--full` flag
  - Change detection using `updated_at` timestamps on all source tables
  - New `precalc_state` table tracks last run and statistics
  - See `docs/INCREMENTAL_PRECALCULATION.md` for details
- Comprehensive release process documentation
- CHANGELOG.md for tracking all project changes
- VERSION file for semantic versioning
- Release guidelines in CONTRIBUTING.md

### Changed
- Graph precalculation service now defaults to incremental mode (was full rebuild)
- Database schema updated with `updated_at` columns on subreddits, users, posts, comments
- Precalculation command accepts `--full` flag to force complete rebuild

## Release Guidelines

When creating a new release:

1. Update the VERSION file with the new version number
2. Update this CHANGELOG.md:
   - Move items from `[Unreleased]` to a new version section
   - Add the release date
   - Create a new empty `[Unreleased]` section at the top
3. Commit the changes with message: `chore: release v{VERSION}`
4. Create and push a git tag: `git tag -a v{VERSION} -m "Release v{VERSION}"`
5. Push the tag: `git push origin v{VERSION}`
6. The GitHub Actions workflow will automatically:
   - Create a GitHub release
   - Generate release notes
   - Build and publish Docker images

### Change Categories

Use these categories when documenting changes:

- **Added** - New features or functionality
- **Changed** - Changes in existing functionality
- **Deprecated** - Soon-to-be removed features
- **Removed** - Removed features
- **Fixed** - Bug fixes
- **Security** - Vulnerability fixes or security improvements
- **Performance** - Performance improvements
- **Documentation** - Documentation updates

### Commit Message Format

To ensure automatic categorization in release notes, use conventional commit prefixes:

- `feat:` - New features (appears in Features section)
- `fix:` - Bug fixes (appears in Bug Fixes section)
- `docs:` - Documentation changes (appears in Documentation section)
- `perf:` - Performance improvements
- `refactor:` - Code refactoring
- `test:` - Adding or updating tests
- `chore:` - Maintenance tasks
- `security:` - Security-related changes

Example: `feat: add support for multiple subreddit crawling`

[Unreleased]: https://github.com/subculture-collective/clustr/compare/v0.1.0...HEAD
