// Package immerle — OpenAPI general information for the native extension API.
//
// @title           immerle extension API
// @version         1.0.0
// @description     Native immerle REST API (first-run setup, capability
// @description     discovery, friends, activity feed, collaborative playlists and
// @description     synchronized Jam sessions) that complements the Subsonic /
// @description     OpenSubsonic API served under /rest/.
// @description
// @description     Served under /api/v1. Authenticated endpoints require a Bearer
// @description     token in the Authorization header — a device JWT (obtained from
// @description     POST /auth/sessions) or a personal API token. Setup, capability
// @description     discovery and session creation are unauthenticated.
// @contact.name    immerle
// @license.name    See repository
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Type "Bearer <token>" where token is a device JWT or API token.
// @tag.name        setup
// @tag.description First-run provisioning of the initial administrator.
// @tag.name        discovery
// @tag.description Capability discovery for client apps.
// @tag.name        friends
// @tag.description Friend requests and listing.
// @tag.name        activity
// @tag.description Social activity feed (privacy-aware).
// @tag.name        playlists
// @tag.description Collaborative playlist management.
// @tag.name        jam
// @tag.description Synchronized listening sessions (SSE).
// @tag.name        tokens
// @tag.description Personal API tokens scoped to the calling user.
// @tag.name        devices
// @tag.description Device sessions (JWT) with a revocation/device registry.
package immerle
