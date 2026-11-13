# Agent Guidelines for Andvari

These instructions apply to all agents and contributors working on this repository.

## 1. Branch & Pull Request Workflow (Mandatory)

- **Always use feature branches:** Never commit or push changes directly to `main`. Create and switch to a descriptive feature branch (e.g., `feat/<feature-name>`, `fix/<issue-name>`).
- **Create a Pull Request:** All changes must be proposed via a Pull Request (PR) targeting `main`.
- **Merge Criteria:** Only merge a PR once all automated tests and CI checks pass on the PR via GitHub Actions (`.github/workflows/build.yml`). Never merge with failing or pending CI checks.

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
