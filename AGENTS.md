# Agent Guidelines for Andvari

These instructions apply to all agents and contributors working on this repository.

## 1. End-to-End Task & Pull Request Workflow (Mandatory)

Do not stop after making local code changes or wait for manual prompting at each lifecycle stage. Complete tasks through the full end-to-end pipeline:

- **Always use feature branches:** Never commit or push changes directly to `main`. Create and switch to a descriptive feature branch (e.g., `feat/<feature-name>`, `fix/<issue-name>`).
- **`Makefile` is canonical:** All build, install, SAST, and test steps live in `Makefile` and nowhere else. `Dockerfile`, GitHub Actions workflows, and local verification must call `Makefile` targets rather than duplicating tool commands.
- **Build & Test:** Run `make test` and `make build` before pushing.
- **Create a Pull Request:** Push the feature branch to `origin` and open a Pull Request targeting `main`.
- **Merge Criteria & Autonomous Merge:** Monitor and wait for all automated tests and CI checks on GitHub Actions (`.github/workflows/build.yml`) to pass. Once checks pass, rebase merge the PR into `main`. Never merge with failing or pending CI checks.
- **Automated releases:** Successful main builds publish the versioned image to GHCR, then tag the commit and create the GitHub release in the same workflow. Changes to application source bump the minor version. Other image build input changes bump the patch version.


## 2. Environment & Tooling

- **Development Tools:** Use Go, Node.js, and npm available on your system. Run development targets through `make`.

## 3. Design & Quality Standards

- Adhere to the design system in `DESIGN.md` and product specifications in `PRODUCT.md`.
- Avoid browser-native dialogs (`alert`, `confirm`, `prompt`); use styled, accessible modal dialogs with keyboard support (`Escape`, backdrop dismiss, focus management).
