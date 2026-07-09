// Mirrors the Go DTOs in desktop/services. Kept hand-written (rather than importing
// the generated wailsjs models) so component code stays decoupled from binding churn.

export interface AssetCounts {
  skills: number
  agents: number
  commands: number
  rules: number
  hooks: number
  plugins: number
}

export interface Profile {
  name: string
  dir: string
  authMethod: string
  createdAt: string
  lastUsed: string
  isDefault: boolean
  counts: AssetCounts
}

export type Layer = 'host' | 'global' | 'profile'

export interface CascadeAsset {
  kind: string
  name: string
  layer: Layer
  source: string
  shadowedIn?: Layer[]
}

export interface CascadeSetting {
  key: string
  layer: Layer
  contributors: Layer[]
  value: string
  merged: boolean
}

export interface Cascade {
  profile: string
  assets: CascadeAsset[]
  settings: CascadeSetting[]
}

export interface UsageTokens {
  input: number
  output: number
  cacheCreation: number
  cacheRead: number
  total: number
}
export interface UsageDay {
  date: string
  total: number
  messages: number
  cost: number
}
export interface UsageNamed {
  name: string
  total: number
  cost: number
}
export interface UsageSession {
  id: string
  cwd: string
  branch: string
  lastTs: string
  total: number
  messages: number
}
export interface Usage {
  profile: string
  window: string
  trackingEnabled: boolean
  totals: UsageTokens
  messages: number
  cost: number
  byDay: UsageDay[]
  byModel: UsageNamed[]
  byProject: UsageNamed[]
  sessions: UsageSession[]
}

export interface Block {
  start: string
  end: string
  lastActivity: string
  total: number
  cost: number
  messages: number
  isActive: boolean
  burnTokensPerMin: number
  costPerHour: number
  remainingMinutes: number
  projectedTotal: number
  projectedCost: number
}

export interface HealthResult {
  available: boolean
  ccpmPath: string
  output: string
  error: string
}

export interface CmdResult {
  ok: boolean
  output: string
  error: string
  ccpmPath: string
}

export interface PermissionView {
  allow: string[]
  ask: string[]
  deny: string[]
  mode: string
}
export interface PluginView {
  name: string
  enabled: boolean
}
export interface EnvVar {
  key: string
  value: string
}
export interface McpView {
  name: string
  type: string
  sources: string[]
}
export interface Details {
  profile: string
  permissions: PermissionView
  plugins: PluginView[]
  env: EnvVar[]
  mcp: McpView[]
}

export interface SettingKV {
  key: string
  value: string
}

export interface UpdateInfo {
  available: boolean
  current: string
  latest: string
  notes: string
  url: string
  assetUrl: string
  assetName: string
}

export interface UpdateProgress {
  phase: string
  percent: number
}
