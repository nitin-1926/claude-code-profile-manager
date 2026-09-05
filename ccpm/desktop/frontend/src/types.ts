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

// --- History -------------------------------------------------------------
// Mirrors ccpm/desktop/services/history.go and ccpm/internal/transcript.

export type BlockKind = 'text' | 'thinking' | 'tool_use' | 'tool_result' | 'image' | 'unknown'

export interface TurnBlock {
  kind: BlockKind
  /** Populated only for `unknown`, so the UI can name what it could not render. */
  rawType?: string
  text?: string
  toolName?: string
  toolUseId?: string
  /** Truncated tool input or output; the full body is fetched on expand. */
  preview?: string
  fullBytes: number
  truncated: boolean
  isError?: boolean
}

export interface Turn {
  index: number
  uuid?: string
  role: 'user' | 'assistant'
  timestamp?: string
  model?: string
  isSidechain: boolean
  isMeta: boolean
  blocks: TurnBlock[]
}

export interface HistorySession {
  id: string
  title: string
  cwd: string
  branch: string
  model: string
  /** Deduped usage-bearing assistant lines — not the reader's turn count. */
  responses: number
  turns: number
  tokens: number
  cost: number
  firstTs: string
  lastTs: string
  /** False when the transcript has been pruned; the row shows but cannot open. */
  openable: boolean
}

export interface HistoryPage {
  turns: Turn[]
  total: number
  offset: number
  unknownBlocks: number
  skippedLines: number
  /** Index a jump-to-turn landed on, or -1. */
  targetIndex: number
}

export interface HistoryToolBody {
  body: string
  fullBytes: number
  truncated: boolean
}

export type HitSource = 'text' | 'tool_use' | 'tool_result'

export interface SearchHit {
  profile: string
  sessionId: string
  title?: string
  cwd?: string
  relPath: string
  mtime: number
  turnUuid?: string
  role: string
  timestamp?: string
  source: HitSource
  toolName?: string
  /** True when the hit is in one of the session's subagent transcripts. */
  subagent: boolean
  /**
   * The snippet arrives pre-split. Go offsets are byte-based and JS strings are
   * UTF-16, so an offset crossing the bridge would mis-highlight any snippet
   * containing a non-ASCII character.
   */
  before: string
  match: string
  after: string
  /** Further matches in this same message beyond the one shown. */
  more: number
}

export interface SearchResult {
  hits: SearchHit[]
  sessions: number
  /** A floor, not a total: scanning stops at each session's quota. */
  matches: number
  truncated: boolean
  droppedSessions: number
  unreadable: number
  cancelled: boolean
}
