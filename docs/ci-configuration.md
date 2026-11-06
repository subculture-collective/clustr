# CI publication boundary

Ordinary Gitea CI retains source, migrated database, browser, image and security qualification. GitHub-specific GHCR publishing and GitHub releases are explicitly restricted to GitHub. They invoke the complete reusable CI graph against the same source revision before publication; a Gitea job token is never used as a GitHub registry credential.

A Gitea publisher is intentionally not introduced. It requires a selected registry, dedicated credential and release policy. No image or release was published while preparing this change.
