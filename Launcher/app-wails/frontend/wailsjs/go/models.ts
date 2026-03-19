export namespace main {
	
	export class AuthState {
	    authenticated: boolean;
	    user_id?: string;
	    username?: string;
	    email?: string;
	    balance_fen?: number;
	    currency?: string;
	    error?: string;
	    warning?: string;
	    agreement_sync_pending?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AuthState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.authenticated = source["authenticated"];
	        this.user_id = source["user_id"];
	        this.username = source["username"];
	        this.email = source["email"];
	        this.balance_fen = source["balance_fen"];
	        this.currency = source["currency"];
	        this.error = source["error"];
	        this.warning = source["warning"];
	        this.agreement_sync_pending = source["agreement_sync_pending"];
	    }
	}
	export class ChatPreflightState {
	    official_model_active: boolean;
	    can_send: boolean;
	    balance_fen?: number;
	    required_balance_fen?: number;
	    low_balance?: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatPreflightState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.official_model_active = source["official_model_active"];
	        this.can_send = source["can_send"];
	        this.balance_fen = source["balance_fen"];
	        this.required_balance_fen = source["required_balance_fen"];
	        this.low_balance = source["low_balance"];
	        this.reason = source["reason"];
	    }
	}
	export class CheckUpdateResult {
	    current: string;
	    available?: string;
	    url?: string;
	    notes?: string;
	    downloaded?: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckUpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.available = source["available"];
	        this.url = source["url"];
	        this.notes = source["notes"];
	        this.downloaded = source["downloaded"];
	        this.error = source["error"];
	    }
	}
	export class OfficialPanelSnapshot {
	    access: platformapi.OfficialAccessState;
	    models: platformapi.OfficialModel[];
	
	    static createFrom(source: any = {}) {
	        return new OfficialPanelSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.access = this.convertValues(source["access"], platformapi.OfficialAccessState);
	        this.models = this.convertValues(source["models"], platformapi.OfficialModel);
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

export namespace platformapi {
	
	export class AgreementDocument {
	    key: string;
	    version: string;
	    title: string;
	    content?: string;
	    url?: string;
	    effective_from_unix?: number;
	
	    static createFrom(source: any = {}) {
	        return new AgreementDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.version = source["version"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.url = source["url"];
	        this.effective_from_unix = source["effective_from_unix"];
	    }
	}
	export class BackendStatus {
	    gateway_url?: string;
	    gateway_healthy: boolean;
	    platform_url?: string;
	    platform_healthy: boolean;
	    settings_url?: string;
	    settings_healthy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BackendStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway_url = source["gateway_url"];
	        this.gateway_healthy = source["gateway_healthy"];
	        this.platform_url = source["platform_url"];
	        this.platform_healthy = source["platform_healthy"];
	        this.settings_url = source["settings_url"];
	        this.settings_healthy = source["settings_healthy"];
	    }
	}
	export class OfficialAccessState {
	    enabled: boolean;
	    balance_fen: number;
	    currency?: string;
	    low_balance: boolean;
	    models_configured?: number;
	    minimum_reserve_fen?: number;
	    minimum_recharge_amount_fen?: number;
	
	    static createFrom(source: any = {}) {
	        return new OfficialAccessState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.balance_fen = source["balance_fen"];
	        this.currency = source["currency"];
	        this.low_balance = source["low_balance"];
	        this.models_configured = source["models_configured"];
	        this.minimum_reserve_fen = source["minimum_reserve_fen"];
	        this.minimum_recharge_amount_fen = source["minimum_recharge_amount_fen"];
	    }
	}
	export class OfficialModel {
	    id: string;
	    name: string;
	    enabled: boolean;
	    pricing_version?: string;
	    reserve_fen?: number;
	
	    static createFrom(source: any = {}) {
	        return new OfficialModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.pricing_version = source["pricing_version"];
	        this.reserve_fen = source["reserve_fen"];
	    }
	}

}

