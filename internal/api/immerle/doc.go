// Package immerle — OpenAPI general information for the native extension API.
//
// @title           immerle extension API
// @version         1.0.0
// @description     Native immerle endpoints (first-run setup, capability
// @description     discovery, friends, activity feed, collaborative playlists and
// @description     synchronized Jam sessions) that complement the Subsonic /
// @description     OpenSubsonic API served under /rest/.
// @description
// @description     Authenticated endpoints reuse Subsonic credentials passed as
// @description     query parameters: u (username) plus either p (password) or the
// @description     token pair t (md5(password+salt)) and s (salt), and c (client
// @description     name). Setup and capability endpoints are unauthenticated.
// @contact.name    immerle
// @license.name    See repository
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
