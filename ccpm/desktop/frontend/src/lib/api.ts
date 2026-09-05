// Single seam between the React app and the generated Wails bindings. Components
// import from here, never from ../wailsjs directly, so binding-path churn and the
// `as unknown as` casts stay in one place.
import { List as ProfilesList, Get as ProfileGet } from '../../wailsjs/go/services/Profiles'
import { Get as CascadeGet } from '../../wailsjs/go/services/CascadeService'
import { Get as UsageGet, Blocks as UsageBlocks } from '../../wailsjs/go/services/UsageService'
import { Doctor } from '../../wailsjs/go/services/HealthService'
import {
  Clone as MClone,
  Rename as MRename,
  Remove as MRemove,
  OpenFolder as MOpenFolder,
  Launch as MLaunch,
  CreateInTerminal as MCreate,
  ImportInTerminal as MImport,
  AddAsset as MAddAsset,
  RemoveAsset as MRemoveAsset,
  AddStdioMCP as MAddStdioMCP,
  AddHTTPMCP as MAddHTTPMCP,
  RemoveMCP as MRemoveMCP,
  TogglePlugin as MTogglePlugin,
  InstallPlugin as MInstallPlugin,
  RemovePlugin as MRemovePlugin,
  SetSetting as MSetSetting,
  AddPermission as MAddPermission,
  RemovePermission as MRemovePermission,
  SetPermissionMode as MSetMode,
  SetEnv as MSetEnv,
  UnsetEnv as MUnsetEnv,
} from '../../wailsjs/go/services/MutateService'
import {
  Sessions as HistorySessions,
  Transcript as HistoryTranscript,
  TranscriptAround as HistoryTranscriptAround,
  ToolBody as HistoryToolBodyFn,
  Search as HistorySearch,
  CancelSearch as HistoryCancelSearch,
  Resume as HistoryResume,
} from '../../wailsjs/go/services/HistoryService'
import { Get as DetailsGet } from '../../wailsjs/go/services/DetailsService'
import { Get as SettingsGet } from '../../wailsjs/go/services/SettingsService'
import { Check as UpdaterCheck, Install as UpdaterInstall } from '../../wailsjs/go/services/Updater'
import { PickDirectory } from '../../wailsjs/go/main/App'
import { EventsOn, BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import type {
  Block,
  Cascade,
  CmdResult,
  Details,
  HealthResult,
  HistoryPage,
  HistorySession,
  HistoryToolBody,
  Profile,
  SearchResult,
  SettingKV,
  UpdateInfo,
  UpdateProgress,
  Usage,
} from '@/types'

export const api = {
  profiles: {
    list: () => ProfilesList() as unknown as Promise<Profile[]>,
    get: (name: string) => ProfileGet(name) as unknown as Promise<Profile | null>,
  },
  cascade: {
    get: (name: string) => CascadeGet(name) as unknown as Promise<Cascade>,
  },
  usage: {
    get: (name: string, window: string) => UsageGet(name, window) as unknown as Promise<Usage>,
    blocks: (name: string) => UsageBlocks(name) as unknown as Promise<Block[]>,
  },
  health: {
    doctor: () => Doctor() as unknown as Promise<HealthResult>,
  },
  history: {
    sessions: (profile: string) => HistorySessions(profile) as unknown as Promise<HistorySession[]>,
    // relPath selects one of the session's subagent transcripts; "" is the
    // session's own. The service matches it against an allowlist built from the
    // index, so this is never an arbitrary path.
    transcript: (profile: string, id: string, relPath: string, offset: number, limit: number) =>
      HistoryTranscript(profile, id, relPath, offset, limit) as unknown as Promise<HistoryPage>,
    transcriptAround: (profile: string, id: string, relPath: string, turnUuid: string, limit: number) =>
      HistoryTranscriptAround(profile, id, relPath, turnUuid, limit) as unknown as Promise<HistoryPage>,
    toolBody: (profile: string, id: string, relPath: string, turnUuid: string, blockIndex: number) =>
      HistoryToolBodyFn(profile, id, relPath, turnUuid, blockIndex) as unknown as Promise<HistoryToolBody>,
    search: (profile: string, query: string, token: string, includeToolResults: boolean) =>
      HistorySearch(profile, query, token, includeToolResults) as unknown as Promise<SearchResult>,
    cancelSearch: (token: string) => HistoryCancelSearch(token) as unknown as Promise<void>,
    resume: (profile: string, id: string) => HistoryResume(profile, id) as unknown as Promise<CmdResult>,
  },
  details: {
    get: (name: string) => DetailsGet(name) as unknown as Promise<Details>,
  },
  settings: {
    get: (name: string) => SettingsGet(name) as unknown as Promise<SettingKV[]>,
  },
  pickDirectory: () => PickDirectory() as unknown as Promise<string>,
  mutate: {
    clone: (src: string, dst: string) => MClone(src, dst) as unknown as Promise<CmdResult>,
    rename: (oldName: string, newName: string) => MRename(oldName, newName) as unknown as Promise<CmdResult>,
    remove: (name: string) => MRemove(name) as unknown as Promise<CmdResult>,
    openFolder: (name: string) => MOpenFolder(name) as unknown as Promise<CmdResult>,
    launch: (name: string) => MLaunch(name) as unknown as Promise<CmdResult>,
    createInTerminal: (name: string) => MCreate(name) as unknown as Promise<CmdResult>,
    importInTerminal: () => MImport() as unknown as Promise<CmdResult>,
    addAsset: (kind: string, path: string, profile: string) =>
      MAddAsset(kind, path, profile) as unknown as Promise<CmdResult>,
    removeAsset: (kind: string, name: string, profile: string) =>
      MRemoveAsset(kind, name, profile) as unknown as Promise<CmdResult>,
    addStdioMCP: (name: string, command: string, profile: string) =>
      MAddStdioMCP(name, command, profile) as unknown as Promise<CmdResult>,
    addHTTPMCP: (name: string, url: string, profile: string) =>
      MAddHTTPMCP(name, url, profile) as unknown as Promise<CmdResult>,
    removeMCP: (name: string, profile: string) => MRemoveMCP(name, profile) as unknown as Promise<CmdResult>,
    togglePlugin: (plugin: string, enable: boolean, profile: string) =>
      MTogglePlugin(plugin, enable, profile) as unknown as Promise<CmdResult>,
    installPlugin: (plugin: string, profile: string) =>
      MInstallPlugin(plugin, profile) as unknown as Promise<CmdResult>,
    removePlugin: (plugin: string, profile: string) =>
      MRemovePlugin(plugin, profile) as unknown as Promise<CmdResult>,
    setSetting: (key: string, value: string, profile: string) =>
      MSetSetting(key, value, profile) as unknown as Promise<CmdResult>,
    addPermission: (bucket: string, rule: string, profile: string) =>
      MAddPermission(bucket, rule, profile) as unknown as Promise<CmdResult>,
    removePermission: (rule: string, profile: string) =>
      MRemovePermission(rule, profile) as unknown as Promise<CmdResult>,
    setPermissionMode: (mode: string, profile: string) =>
      MSetMode(mode, profile) as unknown as Promise<CmdResult>,
    setEnv: (kv: string, profile: string) => MSetEnv(kv, profile) as unknown as Promise<CmdResult>,
    unsetEnv: (key: string, profile: string) => MUnsetEnv(key, profile) as unknown as Promise<CmdResult>,
  },
  updater: {
    check: () => UpdaterCheck() as unknown as Promise<UpdateInfo>,
    install: () => UpdaterInstall() as unknown as Promise<void>,
  },
  // Subscribe to the Go watcher's debounced change signal. Returns an unsubscribe fn.
  onChanged: (cb: () => void): (() => void) => EventsOn('ccpm:changed', cb),
  // Subscribe to updater download/install progress. Returns an unsubscribe fn.
  onUpdateProgress: (cb: (p: UpdateProgress) => void): (() => void) =>
    EventsOn('updater:progress', cb as (...args: unknown[]) => void),
  openURL: (url: string) => BrowserOpenURL(url),
}
