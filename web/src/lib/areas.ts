import type { Task } from './api';

/** The part of life a task belongs to. Mirrors domain.Areas in Go. */
export type Area = 'huji' | 'uoft' | 'home' | 'personal' | 'projects';

export interface AreaInfo {
	id: Area;
	label: string;
	/** Each area's own color. Mid-tone, so its dot reads on light and dark themes alike. */
	color: string;
}

/** Every area, in the order they're shown. */
export const areas: AreaInfo[] = [
	{ id: 'huji', label: 'HUJI', color: '#3b82f6' },
	{ id: 'uoft', label: 'UofT', color: '#8b5cf6' },
	{ id: 'home', label: 'Home / life admin', color: '#f59e0b' },
	{ id: 'personal', label: 'Personal & social', color: '#ec4899' },
	{ id: 'projects', label: 'Side projects', color: '#10b981' }
];

/** The area with this id, or undefined for none or one the client doesn't know. */
export function areaOf(id: string | null | undefined): AreaInfo | undefined {
	return areas.find((a) => a.id === id);
}

/** Reads an area from a query value. Anything else means every area. */
export function parseArea(value: string | null): Area | '' {
	return areaOf(value)?.id ?? '';
}

/** The tasks in an area, or all of them when area is empty. */
export function inArea(tasks: Task[], area: Area | ''): Task[] {
	return area ? tasks.filter((t) => t.area === area) : tasks;
}
