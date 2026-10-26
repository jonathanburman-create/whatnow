import { expect, fake, setUp, startDay, test } from './fixtures';

// The phone layout: a tab bar along the bottom, one-line cards, a + button for capture, and the
// plan's note at the top of Today.
test.use({ viewport: { width: 390, height: 844 } });

test.beforeEach(async ({ request }) => {
	await setUp(request);
	await startDay(request, 'buy milk and call mom');
});

test('Today on a phone: the note shows, cards are one line until tapped, and the strip hides on an empty day', async ({
	page
}) => {
	await page.goto('/');
	await expect(page.getByRole('button', { name: fake.opening })).toBeVisible();
	await expect(page.getByLabel("Today's events")).toBeHidden();
	// The capture box moved to the + button.
	await expect(page.getByLabel('Capture')).toBeHidden();

	const card = page.locator('article').filter({ hasText: 'buy milk' });
	const box = await card.boundingBox();
	expect(box!.height).toBeLessThan(60);

	await card.getByText('buy milk').click();
	await expect(card.getByText('Next in line.')).toBeVisible();
	expect((await card.boundingBox())!.height).toBeGreaterThan(box!.height);
});

test('the tab bar moves between pages and opens the chat', async ({ page }) => {
	await page.goto('/');
	const tabs = page.getByRole('navigation', { name: 'Pages' });

	await tabs.getByRole('link', { name: 'Tasks' }).click();
	await expect(page).toHaveURL(/\/tasks$/);
	await expect(page.getByRole('tab', { name: 'Open' })).toBeVisible();

	await tabs.getByRole('link', { name: 'Ideas' }).click();
	await expect(page).toHaveURL(/\/ideas$/);
	// Ideas has its own box, so the + button stays away.
	await expect(page.getByLabel('Add a task or note')).toBeHidden();

	await tabs.getByRole('button', { name: 'Chat' }).click();
	await expect(page.getByLabel('Message')).toBeVisible();
	await page.getByLabel('Collapse chat').click();
	await expect(page.getByLabel('Message')).toBeHidden();

	await tabs.getByRole('link', { name: 'Today' }).click();
	await expect(page).toHaveURL(/\/$/);
});

test('the + button captures, then gets out of the way', async ({ page }) => {
	await page.goto('/');
	await page.getByLabel('Add a task or note').click();
	const sheet = page.getByRole('dialog', { name: 'Add' });
	await expect(sheet.getByLabel('Capture')).toBeFocused();
	await sheet.getByLabel('Capture').fill('feed the cat');
	await sheet.getByLabel('Capture').press('Enter');

	await expect(sheet).toBeHidden();
	await expect(page.getByRole('status').filter({ hasText: 'Captured' })).toBeVisible();
	await page.getByRole('navigation', { name: 'Pages' }).getByRole('link', { name: 'Tasks' }).click();
	await expect(page.locator('article').filter({ hasText: 'feed the cat' })).toBeVisible();
});

test('could starts folded on a phone and opens on a tap', async ({ page, request }) => {
	await request.post('/api/v1/tasks', { data: { raw_text: 'water the plants' } });
	await request.post('/api/v1/today/replan', { headers: { 'Content-Type': 'application/json' }, data: {} });
	await page.goto('/');
	const fold = page.getByRole('button', { name: /could/i });
	await expect(fold).toHaveAttribute('aria-expanded', 'false');
	await expect(page.getByText('water the plants')).toBeHidden();
	await fold.click();
	await expect(page.getByText('water the plants')).toBeVisible();
});
