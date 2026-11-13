# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users
Self-hosters, homelab operators, small teams, and privacy-conscious organizations running personal or shared cloud storage who want multi-user isolation without operational overhead.

## Product Purpose
Deliver a lightweight, multi-tenant WebDAV storage solution with dead-simple OIDC integration. Andvari exists to give teams and homelabs isolated multi-user storage without the operational burden, resource footprint, and maintenance overhead of heavyweight platforms like Nextcloud, delegating identity seamlessly to modern OIDC/SSO providers.

## Positioning
Andvari is defined by three core pillars: **lightweight performance**, **native multi-tenancy**, and **simple OIDC integration**. While existing WebDAV solutions are either single-user utilities or bloated enterprise suites requiring dedicated databases and complex directory services, Andvari provides isolated multi-user storage that plugs effortlessly into modern OIDC identity stacks with near-zero configuration.

## Operating Context
- Deployed behind or alongside modern OIDC identity providers and reverse proxies (Authentik, Authelia, Keycloak, Tailscale, Cloudflare Access) that handle user authentication.
- Serves multiple independent tenants with strict separation, with admin impersonation available for troubleshooting.
- Primary client access occurs via native WebDAV mounts and sync tools, while the web UI provides token lifecycle management and basic file inspection.

## Capabilities and Constraints
- **Lightweight footprint**: Minimal CPU/memory consumption with no external database dependencies or background worker queues.
- **True multi-tenancy**: Strict storage sandboxing per tenant (`/data/{username}`); cross-tenant data traversal is structurally prevented. Includes admin impersonation for tenant support.
- **Simple OIDC integration**: Effortless integration with modern OIDC identity providers via reverse proxy authentication headers, eliminating internal user registries and credential storage.
- **Dual authentication model**: SSO/OIDC session for the web UI; per-user HTTP Basic Auth tokens for WebDAV client connections.
- **Zero-downtime token rotation**: Maximum 2 concurrent active tokens per tenant for rotation without client disconnects.

## Brand Commitments
- **Name**: Andvari (Norse mythical dwarf guardian of subterranean treasure).
- **Personality**: Lightweight, dependable, unobtrusive, engineered for stability and minimal operational friction.
- **Voice**: Pragmatic, direct, technical, and helpful without unnecessary marketing flourish.

## Evidence on Hand
- Complete Go backend implementation with token persistence and RFC 4918 WebDAV handlers.
- Embedded React 19 + TypeScript + Tailwind CSS dashboard ([`web/src`](web/src)).
- Test suite and setup documentation in [`README.md`](README.md).

## Product Principles
1. **Lightweight Above All**: Operational simplicity and minimal resource usage are paramount; avoid bloat, heavy background services, and unnecessary dependencies.
2. **First-Class Multi-Tenancy**: Every tenant is strictly isolated by default. Multi-user partitioning is an architectural foundation, not an afterthought.
3. **Turnkey OIDC Integration**: Authentication delegates directly to existing OIDC identity providers; setup must remain dead-simple without managing local user databases.
4. **Unobtrusive Management**: The web interface exists strictly to facilitate access, manage tokens, and support multi-tenant administration without getting in the way of everyday workflows.

## Accessibility & Inclusion
- Semantic HTML and WCAG 2.1 AA keyboard navigability across all dashboard modals and tables.
- Accessible ARIA labels on all icon-only actions and focus traps with keyboard `Escape` dismissal on all dialogs.
