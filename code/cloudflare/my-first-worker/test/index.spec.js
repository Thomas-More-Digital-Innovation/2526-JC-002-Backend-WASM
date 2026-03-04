import { env, createExecutionContext, waitOnExecutionContext, SELF } from 'cloudflare:test';
import { describe, it, expect, beforeAll } from 'vitest';
import worker from '../src';

// Seed the D1 database before tests run.
beforeAll(async () => {
	await env.DB.batch([
		env.DB.prepare("CREATE TABLE IF NOT EXISTS statuses (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE)"),
		env.DB.prepare("INSERT OR IGNORE INTO statuses (name) VALUES ('todo')"),
		env.DB.prepare("INSERT OR IGNORE INTO statuses (name) VALUES ('in_progress')"),
		env.DB.prepare("INSERT OR IGNORE INTO statuses (name) VALUES ('done')"),
		env.DB.prepare("CREATE TABLE IF NOT EXISTS todos (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, description TEXT NOT NULL UNIQUE, status_id INTEGER NOT NULL REFERENCES statuses(id) ON DELETE RESTRICT)"),
	]);
});

describe('Go WASI worker', () => {
	it('GET /ping returns pong (unit style)', async () => {
		const request = new Request('http://example.com/ping');
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, env, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(200);
		expect(await response.json()).toEqual({ message: 'pong' });
	});

	it('GET /ping returns pong (integration style)', async () => {
		const response = await SELF.fetch('http://example.com/ping');
		expect(response.status).toBe(200);
		expect(await response.json()).toEqual({ message: 'pong' });
	});

	it('GET /todos returns an array', async () => {
		const response = await SELF.fetch('http://example.com/todos');
		expect(response.status).toBe(200);
		const body = await response.json();
		expect(body).toHaveProperty('todos');
		expect(Array.isArray(body.todos)).toBe(true);
	});

	it('POST /new-todo creates a todo', async () => {
		const response = await SELF.fetch('http://example.com/new-todo', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({
				title: 'Test todo',
				description: 'A unique test description ' + Date.now(),
				status: { name: 'todo' },
			}),
		});
		expect(response.status).toBe(201);
		const body = await response.json();
		expect(body.message).toBe('Todo created');
		expect(body.todo).toBeDefined();
		expect(body.todo.title).toBe('Test todo');
		expect(body.todo.status.name).toBe('todo');
	});

	it('POST /new-todo rejects missing status.name', async () => {
		const response = await SELF.fetch('http://example.com/new-todo', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ title: 'Bad', description: 'bad', status: { name: '' } }),
		});
		expect(response.status).toBe(400);
		const body = await response.json();
		expect(body.error).toContain('status.name is required');
	});

	it('DELETE /todos/:id returns 404 for non-existent todo', async () => {
		const response = await SELF.fetch('http://example.com/todos/999999', {
			method: 'DELETE',
		});
		expect(response.status).toBe(404);
	});

	it('GET /unknown returns 404', async () => {
		const response = await SELF.fetch('http://example.com/unknown');
		expect(response.status).toBe(404);
		const body = await response.json();
		expect(body.error).toBe('not found');
	});
});
