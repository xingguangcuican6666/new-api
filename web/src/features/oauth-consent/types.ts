/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// ============================================================================
// OAuth Consent Types
//
// Describe the authorization request a third-party application makes against
// new-api's built-in OAuth 2.0 / OpenID Connect authorization server. The
// browser lands here from GET /oauth2/authorize with an opaque request token.
// ============================================================================

export interface OAuthConsentClient {
  name: string
  description: string
  logo: string
  homepage: string
}

export interface OAuthConsentScope {
  name: string
  title: string
  description: string
}

export interface OAuthConsentContext {
  client: OAuthConsentClient
  scopes: OAuthConsentScope[]
  already_authorized: boolean
}

export interface OAuthConsentRedirect {
  redirect_uri: string
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}
