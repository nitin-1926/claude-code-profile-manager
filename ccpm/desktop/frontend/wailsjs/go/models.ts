export namespace services {
	
	export class AssetCounts {
	    skills: number;
	    agents: number;
	    commands: number;
	    rules: number;
	    hooks: number;
	    plugins: number;
	
	    static createFrom(source: any = {}) {
	        return new AssetCounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.skills = source["skills"];
	        this.agents = source["agents"];
	        this.commands = source["commands"];
	        this.rules = source["rules"];
	        this.hooks = source["hooks"];
	        this.plugins = source["plugins"];
	    }
	}
	export class CascadeSetting {
	    key: string;
	    layer: string;
	    contributors: string[];
	    value: string;
	    merged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CascadeSetting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.layer = source["layer"];
	        this.contributors = source["contributors"];
	        this.value = source["value"];
	        this.merged = source["merged"];
	    }
	}
	export class CascadeAsset {
	    kind: string;
	    name: string;
	    layer: string;
	    source: string;
	    shadowedIn?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CascadeAsset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.layer = source["layer"];
	        this.source = source["source"];
	        this.shadowedIn = source["shadowedIn"];
	    }
	}
	export class Cascade {
	    profile: string;
	    assets: CascadeAsset[];
	    settings: CascadeSetting[];
	
	    static createFrom(source: any = {}) {
	        return new Cascade(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.assets = this.convertValues(source["assets"], CascadeAsset);
	        this.settings = this.convertValues(source["settings"], CascadeSetting);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class CmdResult {
	    ok: boolean;
	    output: string;
	    error: string;
	    ccpmPath: string;
	
	    static createFrom(source: any = {}) {
	        return new CmdResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.ccpmPath = source["ccpmPath"];
	    }
	}
	export class McpView {
	    name: string;
	    type: string;
	    sources: string[];
	
	    static createFrom(source: any = {}) {
	        return new McpView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.sources = source["sources"];
	    }
	}
	export class EnvVar {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvVar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class PluginView {
	    name: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PluginView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	    }
	}
	export class PermissionView {
	    allow: string[];
	    ask: string[];
	    deny: string[];
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new PermissionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allow = source["allow"];
	        this.ask = source["ask"];
	        this.deny = source["deny"];
	        this.mode = source["mode"];
	    }
	}
	export class Details {
	    profile: string;
	    permissions: PermissionView;
	    plugins: PluginView[];
	    env: EnvVar[];
	    mcp: McpView[];
	
	    static createFrom(source: any = {}) {
	        return new Details(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.permissions = this.convertValues(source["permissions"], PermissionView);
	        this.plugins = this.convertValues(source["plugins"], PluginView);
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.mcp = this.convertValues(source["mcp"], McpView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class HealthResult {
	    available: boolean;
	    ccpmPath: string;
	    output: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.ccpmPath = source["ccpmPath"];
	        this.output = source["output"];
	        this.error = source["error"];
	    }
	}
	export class HistoryPage {
	    turns: transcript.Turn[];
	    total: number;
	    offset: number;
	    unknownBlocks: number;
	    skippedLines: number;
	    targetIndex: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.turns = this.convertValues(source["turns"], transcript.Turn);
	        this.total = source["total"];
	        this.offset = source["offset"];
	        this.unknownBlocks = source["unknownBlocks"];
	        this.skippedLines = source["skippedLines"];
	        this.targetIndex = source["targetIndex"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HistorySession {
	    id: string;
	    title: string;
	    cwd: string;
	    branch: string;
	    model: string;
	    responses: number;
	    turns: number;
	    tokens: number;
	    cost: number;
	    firstTs: string;
	    lastTs: string;
	    openable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistorySession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.cwd = source["cwd"];
	        this.branch = source["branch"];
	        this.model = source["model"];
	        this.responses = source["responses"];
	        this.turns = source["turns"];
	        this.tokens = source["tokens"];
	        this.cost = source["cost"];
	        this.firstTs = source["firstTs"];
	        this.lastTs = source["lastTs"];
	        this.openable = source["openable"];
	    }
	}
	export class HistoryToolBody {
	    body: string;
	    fullBytes: number;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryToolBody(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.body = source["body"];
	        this.fullBytes = source["fullBytes"];
	        this.truncated = source["truncated"];
	    }
	}
	
	
	
	export class Profile {
	    name: string;
	    dir: string;
	    authMethod: string;
	    createdAt: string;
	    lastUsed: string;
	    isDefault: boolean;
	    counts: AssetCounts;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dir = source["dir"];
	        this.authMethod = source["authMethod"];
	        this.createdAt = source["createdAt"];
	        this.lastUsed = source["lastUsed"];
	        this.isDefault = source["isDefault"];
	        this.counts = this.convertValues(source["counts"], AssetCounts);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SettingKV {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingKV(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class UpdateInfo {
	    available: boolean;
	    current: string;
	    latest: string;
	    notes: string;
	    url: string;
	    assetUrl: string;
	    assetName: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.notes = source["notes"];
	        this.url = source["url"];
	        this.assetUrl = source["assetUrl"];
	        this.assetName = source["assetName"];
	    }
	}
	export class UsageSession {
	    id: string;
	    cwd: string;
	    branch: string;
	    lastTs: string;
	    total: number;
	    messages: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.cwd = source["cwd"];
	        this.branch = source["branch"];
	        this.lastTs = source["lastTs"];
	        this.total = source["total"];
	        this.messages = source["messages"];
	    }
	}
	export class UsageNamed {
	    name: string;
	    total: number;
	    cost: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageNamed(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.total = source["total"];
	        this.cost = source["cost"];
	    }
	}
	export class UsageDay {
	    date: string;
	    total: number;
	    messages: number;
	    cost: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageDay(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.total = source["total"];
	        this.messages = source["messages"];
	        this.cost = source["cost"];
	    }
	}
	export class UsageTokens {
	    input: number;
	    output: number;
	    cacheCreation: number;
	    cacheRead: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageTokens(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input = source["input"];
	        this.output = source["output"];
	        this.cacheCreation = source["cacheCreation"];
	        this.cacheRead = source["cacheRead"];
	        this.total = source["total"];
	    }
	}
	export class Usage {
	    profile: string;
	    window: string;
	    trackingEnabled: boolean;
	    totals: UsageTokens;
	    messages: number;
	    cost: number;
	    byDay: UsageDay[];
	    byModel: UsageNamed[];
	    byProject: UsageNamed[];
	    sessions: UsageSession[];
	
	    static createFrom(source: any = {}) {
	        return new Usage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.window = source["window"];
	        this.trackingEnabled = source["trackingEnabled"];
	        this.totals = this.convertValues(source["totals"], UsageTokens);
	        this.messages = source["messages"];
	        this.cost = source["cost"];
	        this.byDay = this.convertValues(source["byDay"], UsageDay);
	        this.byModel = this.convertValues(source["byModel"], UsageNamed);
	        this.byProject = this.convertValues(source["byProject"], UsageNamed);
	        this.sessions = this.convertValues(source["sessions"], UsageSession);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	

}

export namespace transcript {
	
	export class Block {
	    kind: string;
	    rawType?: string;
	    text?: string;
	    toolName?: string;
	    toolUseId?: string;
	    preview?: string;
	    fullBytes: number;
	    truncated: boolean;
	    isError?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Block(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.rawType = source["rawType"];
	        this.text = source["text"];
	        this.toolName = source["toolName"];
	        this.toolUseId = source["toolUseId"];
	        this.preview = source["preview"];
	        this.fullBytes = source["fullBytes"];
	        this.truncated = source["truncated"];
	        this.isError = source["isError"];
	    }
	}
	export class Hit {
	    profile: string;
	    sessionId: string;
	    title?: string;
	    cwd?: string;
	    relPath: string;
	    mtime: number;
	    turnUuid?: string;
	    role: string;
	    timestamp?: string;
	    source: string;
	    toolName?: string;
	    before: string;
	    match: string;
	    after: string;
	    more: number;
	
	    static createFrom(source: any = {}) {
	        return new Hit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.sessionId = source["sessionId"];
	        this.title = source["title"];
	        this.cwd = source["cwd"];
	        this.relPath = source["relPath"];
	        this.mtime = source["mtime"];
	        this.turnUuid = source["turnUuid"];
	        this.role = source["role"];
	        this.timestamp = source["timestamp"];
	        this.source = source["source"];
	        this.toolName = source["toolName"];
	        this.before = source["before"];
	        this.match = source["match"];
	        this.after = source["after"];
	        this.more = source["more"];
	    }
	}
	export class SearchResult {
	    hits: Hit[];
	    sessions: number;
	    matches: number;
	    truncated: boolean;
	    droppedSessions: number;
	    unreadable: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hits = this.convertValues(source["hits"], Hit);
	        this.sessions = source["sessions"];
	        this.matches = source["matches"];
	        this.truncated = source["truncated"];
	        this.droppedSessions = source["droppedSessions"];
	        this.unreadable = source["unreadable"];
	        this.cancelled = source["cancelled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Turn {
	    index: number;
	    uuid?: string;
	    role: string;
	    timestamp?: string;
	    model?: string;
	    isSidechain: boolean;
	    isMeta: boolean;
	    blocks: Block[];
	
	    static createFrom(source: any = {}) {
	        return new Turn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.uuid = source["uuid"];
	        this.role = source["role"];
	        this.timestamp = source["timestamp"];
	        this.model = source["model"];
	        this.isSidechain = source["isSidechain"];
	        this.isMeta = source["isMeta"];
	        this.blocks = this.convertValues(source["blocks"], Block);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace usage {
	
	export class Tokens {
	    input: number;
	    output: number;
	    cache_creation: number;
	    cache_read: number;
	
	    static createFrom(source: any = {}) {
	        return new Tokens(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input = source["input"];
	        this.output = source["output"];
	        this.cache_creation = source["cache_creation"];
	        this.cache_read = source["cache_read"];
	    }
	}
	export class Block {
	    start: string;
	    end: string;
	    lastActivity: string;
	    tokens: Tokens;
	    total: number;
	    cost: number;
	    messages: number;
	    isActive: boolean;
	    burnTokensPerMin: number;
	    costPerHour: number;
	    remainingMinutes: number;
	    projectedTotal: number;
	    projectedCost: number;
	
	    static createFrom(source: any = {}) {
	        return new Block(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.lastActivity = source["lastActivity"];
	        this.tokens = this.convertValues(source["tokens"], Tokens);
	        this.total = source["total"];
	        this.cost = source["cost"];
	        this.messages = source["messages"];
	        this.isActive = source["isActive"];
	        this.burnTokensPerMin = source["burnTokensPerMin"];
	        this.costPerHour = source["costPerHour"];
	        this.remainingMinutes = source["remainingMinutes"];
	        this.projectedTotal = source["projectedTotal"];
	        this.projectedCost = source["projectedCost"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

