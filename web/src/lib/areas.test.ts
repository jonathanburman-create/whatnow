import { describe, expect, it } from 'vitest';
import type { Task } from './api';
import { areaOf, inArea, parseArea } from './areas';

const task = (id: number, area: Task['area']): Task => ({
	id,
	title: `task ${id}`,
	notes: '',
	raw_text: '',
	size: null,
	energy: null,
	area,
	feeling: '',
	due_on: null,
	not_before: null,
	status: 'open',
	refs: null,
	created_at: '2026-10-25T12:00:00Z',
	updated_at: '2026-10-25T12:00:00Z'
});

describe('areas', () => {
	it('Should keep only the tasks in the picked area', () => {
		const tasks = [task(1, 'uoft'), task(2, 'home'), task(3, null), task(4, 'uoft')];
		expect(inArea(tasks, 'uoft').map((t) => t.id)).toEqual([1, 4]);
	});

	it('Should keep every task when no area is picked', () => {
		const tasks = [task(1, 'uoft'), task(2, null)];
		expect(inArea(tasks, '')).toEqual(tasks);
	});

	it('Should read a known area from the address and ignore anything else', () => {
		expect(parseArea('huji')).toBe('huji');
		expect(parseArea('nope')).toBe('');
		expect(parseArea(null)).toBe('');
	});

	it('Should find an area by id, and nothing for none', () => {
		expect(areaOf('personal')?.label).toBe('Personal & social');
		expect(areaOf(null)).toBeUndefined();
	});
});
