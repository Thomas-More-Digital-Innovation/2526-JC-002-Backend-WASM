import wasmModule from '../build/main.wasm';
import { WASI, File, OpenFile, ConsoleStdout } from '@bjorn3/browser_wasi_shim';

const textDecoder = new TextDecoder();

// ---------------------------------------------------------------------------
// WAGI host – invoke the Go WASM module with CGI env-vars / stdin / stdout
// ---------------------------------------------------------------------------

function concatChunks(chunks) {
	const totalLength = chunks.reduce((sum, c) => sum + c.byteLength, 0);
	const merged = new Uint8Array(totalLength);
	let offset = 0;
	for (const chunk of chunks) {
		merged.set(chunk, offset);
		offset += chunk.byteLength;
	}
	return merged;
}

function buildEnvArray(request, bodySize) {
	const url = new URL(request.url);
	const env = [
		`REQUEST_METHOD=${request.method}`,
		`PATH_INFO=${url.pathname || '/'}`,
		`QUERY_STRING=${url.search.length > 1 ? url.search.slice(1) : ''}`,
		`REQUEST_URI=${url.pathname}${url.search}`,
		`CONTENT_LENGTH=${bodySize}`,
	];

	const ct = request.headers.get('content-type');
	if (ct) env.push(`CONTENT_TYPE=${ct}`);

	return env;
}

function parseWagiOutput(stdoutChunks) {
	const raw = textDecoder.decode(concatChunks(stdoutChunks));
	const sepCRLF = raw.indexOf('\r\n\r\n');
	const sepLF = raw.indexOf('\n\n');
	const sepIdx = sepCRLF >= 0 ? sepCRLF : sepLF;

	if (sepIdx < 0) {
		return { status: 500, headers: new Headers({ 'content-type': 'text/plain' }), body: raw, dbAction: null };
	}

	const sepSize = sepCRLF >= 0 ? 4 : 2;
	const headerLines = raw.slice(0, sepIdx).split(/\r?\n/);
	const body = raw.slice(sepIdx + sepSize);

	let status = 200;
	const headers = new Headers();
	let dbAction = null;

	for (const line of headerLines) {
		const colon = line.indexOf(':');
		if (colon < 0) continue;
		const key = line.slice(0, colon).trim();
		const value = line.slice(colon + 1).trim();
		if (!key || !value) continue;

		if (key.toLowerCase() === 'status') {
			const code = Number.parseInt(value, 10);
			if (Number.isInteger(code) && code >= 100 && code <= 599) status = code;
			continue;
		}

		if (key.toLowerCase() === 'x-db-action') {
			try { dbAction = JSON.parse(value); } catch { /* ignore */ }
			continue;
		}

		headers.append(key, value);
	}

	if (!headers.has('content-type')) {
		headers.set('content-type', 'application/json; charset=utf-8');
	}

	return { status, headers, body, dbAction };
}

async function invokeWasm(request) {
	const requestBytes = new Uint8Array(await request.arrayBuffer());
	const stdoutChunks = [];
	const stderrChunks = [];

	const wasi = new WASI(
		['api-wasm'],
		buildEnvArray(request, requestBytes.byteLength),
		[
			new OpenFile(new File(requestBytes)),
			new ConsoleStdout((chunk) => stdoutChunks.push(chunk)),
			new ConsoleStdout((chunk) => stderrChunks.push(chunk)),
		],
		{ debug: false },
	);

	const instance = await WebAssembly.instantiate(wasmModule, {
		wasi_snapshot_preview1: wasi.wasiImport,
	});

	const exitCode = wasi.start(instance);

	if (exitCode !== 0) {
		const stderr = textDecoder.decode(concatChunks(stderrChunks));
		return {
			status: 500,
			headers: new Headers({ 'content-type': 'application/json' }),
			body: JSON.stringify({ error: 'wasm exited with non-zero code', exitCode, details: stderr || undefined }),
			dbAction: null,
		};
	}

	return parseWagiOutput(stdoutChunks);
}

// ---------------------------------------------------------------------------
// Auto-migration – runs once per cold start, idempotent
// ---------------------------------------------------------------------------

let migrationsApplied = false;

async function ensureMigrations(db) {
	if (migrationsApplied) return;

	await db.batch([
		db.prepare(`CREATE TABLE IF NOT EXISTS statuses (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT    NOT NULL UNIQUE
		)`),
		db.prepare("INSERT OR IGNORE INTO statuses (name) VALUES ('todo')"),
		db.prepare("INSERT OR IGNORE INTO statuses (name) VALUES ('in_progress')"),
		db.prepare("INSERT OR IGNORE INTO statuses (name) VALUES ('done')"),
		db.prepare(`CREATE TABLE IF NOT EXISTS todos (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			title       TEXT    NOT NULL,
			description TEXT    NOT NULL UNIQUE,
			status_id   INTEGER NOT NULL REFERENCES statuses(id) ON DELETE RESTRICT
		)`),
	]);

	migrationsApplied = true;
}

// ---------------------------------------------------------------------------
// D1 database operations
// ---------------------------------------------------------------------------

function todoRow(row) {
	return {
		id: row.id,
		title: row.title,
		description: row.description,
		statusId: row.status_id,
		status: { id: row.status_id, name: row.status_name },
	};
}

const TODO_SELECT = `
	SELECT t.id, t.title, t.description, t.status_id,
	       s.name AS status_name
	FROM todos t
	JOIN statuses s ON s.id = t.status_id`;

async function dbListTodos(db) {
	const { results } = await db.prepare(`${TODO_SELECT} ORDER BY t.id ASC`).all();
	return jsonResponse(200, { todos: results.map(todoRow) });
}

async function dbCreateTodo(db, params) {
	// Upsert status
	await db.prepare('INSERT INTO statuses (name) VALUES (?) ON CONFLICT (name) DO NOTHING')
		.bind(params.statusName).run();
	const status = await db.prepare('SELECT id FROM statuses WHERE name = ?')
		.bind(params.statusName).first();

	// Insert todo
	try {
		await db.prepare('INSERT INTO todos (title, description, status_id) VALUES (?, ?, ?)')
			.bind(params.title, params.description, status.id).run();
	} catch (err) {
		if (isUniqueError(err)) {
			return jsonResponse(409, { error: 'todo description already exists' });
		}
		throw err;
	}

	const row = await db.prepare(`${TODO_SELECT} WHERE t.description = ? AND t.status_id = ? ORDER BY t.id DESC LIMIT 1`)
		.bind(params.description, status.id).first();

	return jsonResponse(201, { message: 'Todo created', todo: todoRow(row) });
}

async function dbUpdateTodo(db, params) {
	// Check existence
	const existing = await db.prepare('SELECT id FROM todos WHERE id = ?').bind(params.id).first();
	if (!existing) return jsonResponse(404, { error: 'todo not found' });

	// Upsert status
	await db.prepare('INSERT INTO statuses (name) VALUES (?) ON CONFLICT (name) DO NOTHING')
		.bind(params.statusName).run();
	const status = await db.prepare('SELECT id FROM statuses WHERE name = ?')
		.bind(params.statusName).first();

	// Update
	try {
		await db.prepare('UPDATE todos SET title = ?, description = ?, status_id = ? WHERE id = ?')
			.bind(params.title, params.description, status.id, params.id).run();
	} catch (err) {
		if (isUniqueError(err)) {
			return jsonResponse(409, { error: 'todo description already exists' });
		}
		throw err;
	}

	const row = await db.prepare(`${TODO_SELECT} WHERE t.id = ?`).bind(params.id).first();
	return jsonResponse(200, { message: 'Todo updated', todo: todoRow(row) });
}

async function dbDeleteTodo(db, params) {
	const { meta } = await db.prepare('DELETE FROM todos WHERE id = ?').bind(params.id).run();
	if (meta.changes === 0) return jsonResponse(404, { error: 'todo not found' });
	return new Response(null, { status: 204 });
}

function isUniqueError(err) {
	const msg = String(err?.message || err || '').toLowerCase();
	return msg.includes('unique') || msg.includes('constraint');
}

function jsonResponse(status, body) {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'content-type': 'application/json' },
	});
}

async function executeDbAction(db, action) {
	switch (action.action) {
		case 'list-todos':  return dbListTodos(db);
		case 'create-todo': return dbCreateTodo(db, action.params);
		case 'update-todo': return dbUpdateTodo(db, action.params);
		case 'delete-todo': return dbDeleteTodo(db, action.params);
		default:
			return jsonResponse(500, { error: `unknown db action: ${action.action}` });
	}
}

// ---------------------------------------------------------------------------
// Worker entrypoint
// ---------------------------------------------------------------------------

export default {
	async fetch(request, env) {
		const wagiResult = await invokeWasm(request);

		// No DB action → WASM produced the complete response (e.g. /ping, validation errors).
		if (!wagiResult.dbAction) {
			return new Response(wagiResult.body, {
				status: wagiResult.status,
				headers: wagiResult.headers,
			});
		}

		// DB action required → ensure tables exist, then execute against D1.
		try {
			await ensureMigrations(env.DB);
			return await executeDbAction(env.DB, wagiResult.dbAction);
		} catch (err) {
			return jsonResponse(500, { error: 'database error', details: String(err?.message || err) });
		}
	},
};
