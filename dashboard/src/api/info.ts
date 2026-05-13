/**
 * Engine capability discovery via GET /info.
 *
 * Note: /info is served at the engine root (not under /api/v1), so this
 * module bypasses the `api` client in `./client.ts` and uses plain fetch.
 */

export type EngineMode = "dev" | "serve" | "multitenant"
export type OAuthProvider = "google" | "github"

/**
 * Capabilities is the typed shape of the /info response that the dashboard
 * cares about. The engine returns additional fields (capabilities array,
 * runtime_config, etc.) which we ignore here.
 */
export interface Capabilities {
  version: string
  goVersion: string
  platform: string
  arch: string
  mode: EngineMode
  platformEnabled: boolean
  authEnabled: boolean
  passwordAuthEnabled: boolean
  oauthProviders: OAuthProvider[]
}

/** Raw wire shape returned by GET /info — snake_case as it comes off Go. */
interface InfoResponseWire {
  version: string
  go_version: string
  platform: string
  arch: string
  mode: EngineMode
  platform_enabled: boolean
  auth_enabled: boolean
  password_auth_enabled: boolean
  oauth_providers: OAuthProvider[]
}

/** Fetch /info and convert snake_case to camelCase. */
export async function fetchInfo(): Promise<Capabilities> {
  const response = await fetch("/info", {
    headers: { Accept: "application/json" },
  })
  if (!response.ok) {
    throw new Error(`GET /info failed: HTTP ${response.status}`)
  }
  const raw = (await response.json()) as InfoResponseWire
  return {
    version: raw.version,
    goVersion: raw.go_version,
    platform: raw.platform,
    arch: raw.arch,
    mode: raw.mode,
    platformEnabled: raw.platform_enabled,
    authEnabled: raw.auth_enabled,
    passwordAuthEnabled: raw.password_auth_enabled,
    oauthProviders: raw.oauth_providers ?? [],
  }
}
