# Agent Guidelines for Andvari

These instructions apply to all agents and contributors working on this repository.

## 1. End-to-End Task & Pull Request Workflow (Mandatory)

Do not stop after making local code changes or wait for manual prompting at each lifecycle stage. Complete tasks through the full end-to-end pipeline:

- **Always use feature branches:** Never commit or push changes directly to `main`. Create and switch to a descriptive feature branch (e.g., `feat/<feature-name>`, `fix/<issue-name>`).
- **Build & Test:** Verify changes thoroughly locally before pushing:
  - Run backend tests: `go test -v -cover ./...`
  - Rebuild web assets: `npm --prefix web run build` (ensure `web/dist/index.html` is updated if frontend files changed).
- **Create a Pull Request:** Push the feature branch to `origin` and open a Pull Request targeting `main` (`gh pr create`).
- **Merge Criteria & Autonomous Merge:** Monitor and wait for all automated tests and CI checks on GitHub Actions (`.github/workflows/build.yml`) to pass. Once checks pass, squash merge the PR into `main` (`gh pr merge --squash --delete-branch`). Never merge with failing or pending CI checks.
- **Automated releases:** Successful main builds publish the versioned image to GHCR, then tag the commit and create the GitHub release in the same workflow. Changes to application source bump the minor version. Other changes bump the patch version.


## 2. Environment & Tooling

- **Nix Shell Environment:** The development tools (Go, Node.js, npm) are managed via `shell.nix`. Run commands via:
  ```bash
  nix-shell --run "<command>"
  ```
- **Backend Testing:**
  ```bash
  nix-shell --run "go test -v -cover ./..."
  ```
- **Frontend Building & Typechecking:**
  ```bash
  nix-shell --run "npm --prefix web ci"
  nix-shell --run "npm --prefix web run build"
  ```

## 3. Design & Quality Standards

- Adhere to the design system in `DESIGN.md` and product specifications in `PRODUCT.md`.
- Avoid browser-native dialogs (`alert`, `confirm`, `prompt`); use styled, accessible modal dialogs with keyboard support (`Escape`, backdrop dismiss, focus management).
