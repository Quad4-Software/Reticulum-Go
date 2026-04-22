// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

"use strict";

console.log("[JS] Starting wasm_exec_node.js...");

if (process.argv.length < 3) {
	console.error("usage: go_js_wasm_exec [wasm binary] [arguments]");
	process.exit(1);
}

globalThis.require = require;
globalThis.fs = require("fs");
globalThis.path = require("path");
globalThis.TextEncoder = require("util").TextEncoder;
globalThis.TextDecoder = require("util").TextDecoder;

globalThis.performance ??= require("performance");

globalThis.crypto ??= require("crypto");

try {
	const WS = require("ws");
	globalThis.WebSocket = class extends WS {
		constructor(url, protocols) {
			console.log(`[JS] WebSocket connecting to: ${url}`);
			super(url, protocols);
			
			// Ensure compatibility with browser-style onopen, onmessage, etc.
			// The superclass (ws.WebSocket) handles these if assigned.
			
			this.on("open", () => {
				console.log(`[JS] WebSocket connected to: ${url}`);
				if (typeof this.onopen === "function") this.onopen();
			});
			this.on("message", (data, isBinary) => {
				if (typeof this.onmessage === "function") {
					// Browser onmessage expects an Event-like object with a 'data' property
					this.onmessage({ data: data });
				}
			});
			this.on("close", (code, reason) => {
				console.log(`[JS] WebSocket closed: ${url} (${code} ${reason})`);
				if (typeof this.onclose === "function") this.onclose({ code, reason });
			});
			this.on("error", (err) => {
				console.error(`[JS] WebSocket error: ${url}`, err);
				if (typeof this.onerror === "function") this.onerror(err);
			});
		}
	};
	console.log("[JS] WebSocket polyfill initialized using 'ws' package");
} catch (e) {
	console.warn("[JS] Warning: 'ws' package not found. WebSocket will be undefined.");
}

require("./wasm_exec");

const go = new Go();
go.argv = process.argv.slice(2);
// Filter environment variables to avoid "total length of command line and environment variables exceeds limit" error
const allowedEnv = ["TMPDIR", "PATH", "HOME", "USER", "LANG"];
const filteredEnv = { TMPDIR: require("os").tmpdir() };
for (const key of allowedEnv) {
	if (process.env[key] !== undefined) {
		filteredEnv[key] = process.env[key];
	}
}
go.env = filteredEnv;
go.exit = process.exit;
WebAssembly.instantiate(fs.readFileSync(process.argv[2]), go.importObject).then((result) => {
	process.on("exit", (code) => { // Node.js exits if no event handler is pending
		if (code === 0 && !go.exited) {
			// deadlock, make Go print error and stack traces
			go._pendingEvent = { id: 0 };
			go._resume();
		}
	});
	return go.run(result.instance);
}).catch((err) => {
	console.error(err);
	process.exit(1);
});
