export namespace main {
	
	export class GifConfig {
	    inputFolder: string;
	    outputPath: string;
	    width: number;
	    height: number;
	    delay: number;
	    loopCount: number;
	    fadeIn: boolean;
	    fadeOut: boolean;
	    fadeDuration: number;
	    scaleMode: string;
	    quality: number;
	    padColor: string;
	
	    static createFrom(source: any = {}) {
	        return new GifConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputFolder = source["inputFolder"];
	        this.outputPath = source["outputPath"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.delay = source["delay"];
	        this.loopCount = source["loopCount"];
	        this.fadeIn = source["fadeIn"];
	        this.fadeOut = source["fadeOut"];
	        this.fadeDuration = source["fadeDuration"];
	        this.scaleMode = source["scaleMode"];
	        this.quality = source["quality"];
	        this.padColor = source["padColor"];
	    }
	}
	export class ImageInfo {
	    name: string;
	    path: string;
	    thumbnail: string;
	    width: number;
	    height: number;
	    index: number;
	
	    static createFrom(source: any = {}) {
	        return new ImageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.thumbnail = source["thumbnail"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.index = source["index"];
	    }
	}

}

