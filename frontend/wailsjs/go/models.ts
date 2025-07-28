export namespace main {
	
	export class AddCardRequest {
	    url: string;
	    type: string;
	    quality: string;
	    language: string;
	    edition: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AddCardRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.type = source["type"];
	        this.quality = source["quality"];
	        this.language = source["language"];
	        this.edition = source["edition"];
	    }
	}
	export class Card {
	    id: number;
	    name: string;
	    set_name: string;
	    rarity: string;
	    price: string;
	    price_num: number;
	    image_url: string;
	    card_url: string;
	    type: string;
	    added_at: string;
	    last_updated: string;
	    quality: string;
	    language: string;
	    edition: boolean;
	    total_offers: number;
	
	    static createFrom(source: any = {}) {
	        return new Card(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.set_name = source["set_name"];
	        this.rarity = source["rarity"];
	        this.price = source["price"];
	        this.price_num = source["price_num"];
	        this.image_url = source["image_url"];
	        this.card_url = source["card_url"];
	        this.type = source["type"];
	        this.added_at = source["added_at"];
	        this.last_updated = source["last_updated"];
	        this.quality = source["quality"];
	        this.language = source["language"];
	        this.edition = source["edition"];
	        this.total_offers = source["total_offers"];
	    }
	}

}

