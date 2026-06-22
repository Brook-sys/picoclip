import { expect, test } from '@playwright/test';

test.describe('PicoClip smoke UI', () => {
  test('loads primary pages without console or HTTP errors', async ({ page }) => {
    const consoleErrors: string[] = [];
    const failedRequests: string[] = [];

    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text());
    });
    page.on('requestfailed', (request) => {
      if (request.url().includes('/sse/')) return;
      failedRequests.push(`${request.method()} ${request.url()}`);
    });

    for (const path of ['/', '/projects', '/agents', '/tasks', '/runs', '/skills', '/activity', '/settings']) {
      const response = await page.goto(path);
      expect(response?.ok(), `${path} should return 2xx`).toBeTruthy();
      await expect(page.locator('main')).toBeVisible();
    }

    expect(consoleErrors).toEqual([]);
    expect(failedRequests).toEqual([]);
  });

  test('creates agent and task, keeps task detail stable during htmx polling', async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text());
    });

    const agentName = `E2E Agent ${Date.now()}`;

    await page.goto('/agents');
    await page.getByTestId('agent-create-button').click();
    await page.locator('[data-modal="agent-quick-modal"]').getByPlaceholder('Nome').fill(agentName);
    await page.locator('[data-modal="agent-quick-modal"] select[name="type"]').selectOption('noop');
    await page.getByTestId('agent-create-submit').click();
    await expect(page.getByRole('heading', { name: agentName })).toBeVisible();

    await page.goto('/tasks');
    await page.getByTestId('task-create-button').click();
    const taskModal = page.locator('[data-modal="task-create-modal"]');
    await taskModal.locator('.agent-search').fill(agentName);
    await taskModal.locator('[data-agent-option]').first().click();
    const taskPrompt = `Validate task detail polling stability ${Date.now()}`;
    await taskModal.getByPlaceholder('e.g., Update database schema').fill(taskPrompt);
    await taskModal.getByPlaceholder('Objetivo, contexto e critérios de aceite').fill(taskPrompt);
    await page.getByTestId('task-create-submit').click();
    await expect(page.getByText(taskPrompt).first()).toBeVisible();

    await page.getByRole('row').filter({ hasText: taskPrompt }).getByRole('link', { name: taskPrompt }).click();
    await expect(page.locator('#task-live')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Wake agent' })).toBeVisible();
    await page.waitForTimeout(3500);
    await expect(page.getByRole('button', { name: 'Wake agent' })).toBeVisible();
    await expect(page.locator('#task-live')).toBeVisible();

    const commentBody = `Please revise with more detail ${Date.now()}.`;
    await page.getByPlaceholder('Add comment or command...').fill(commentBody);
    await page.getByRole('button', { name: 'Post comment' }).click();
    await expect(page.locator('#task-live article.message').filter({ hasText: commentBody })).toBeVisible();

    expect(consoleErrors).toEqual([]);
  });

  test('command palette searches and navigates', async ({ page }) => {
    await page.goto('/');
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K');
    await expect(page.getByPlaceholder('Type a command or search...')).toBeVisible();
    await page.getByPlaceholder('Type a command or search...').fill('open settings');
    await expect(page.locator('.command-item').filter({ hasText: 'Open Settings' })).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL('/settings');
  });

  test('diagnostics API exposes health checks', async ({ request }) => {
    const response = await request.get('/api/diagnostics');
    expect(response.ok()).toBeTruthy();
    const diagnostics = await response.json();
    expect(diagnostics.checks.length).toBeGreaterThan(0);
    expect(diagnostics.checks.some((check: { name: string }) => check.name === 'storage_read')).toBeTruthy();
  });

  test('agent API exposes Paperclip-like task workflow', async ({ request }) => {
    const agentResponse = await request.post('/api/agents', {
      data: { name: `API Agent ${Date.now()}`, type: 'noop' },
    });
    expect(agentResponse.ok()).toBeTruthy();
    const agent = await agentResponse.json();

    const taskResponse = await request.post('/agent-api/tasks', {
      data: { assignee_agent_id: agent.id, prompt: 'API lifecycle task' },
    });
    expect(taskResponse.ok()).toBeTruthy();
    const task = await taskResponse.json();
    expect(task.status).toBe('todo');

    const checkoutResponse = await request.post(`/agent-api/tasks/${task.id}/checkout`, {
      data: { agent_id: agent.id, expected_statuses: ['todo'] },
    });
    expect(checkoutResponse.ok()).toBeTruthy();
    expect((await checkoutResponse.json()).status).toBe('in_progress');

    const blockedResponse = await request.patch(`/agent-api/tasks/${task.id}`, {
      data: { agent_id: agent.id, status: 'blocked', comment: 'Blocked for E2E validation.' },
    });
    expect(blockedResponse.ok()).toBeTruthy();
    expect((await blockedResponse.json()).status).toBe('blocked');

    const commentResponse = await request.post(`/agent-api/tasks/${task.id}/comments`, {
      data: { role: 'user', body: 'Unblocked, continue.' },
    });
    expect(commentResponse.ok()).toBeTruthy();

    const detailResponse = await request.get(`/agent-api/tasks/${task.id}`);
    expect(detailResponse.ok()).toBeTruthy();
    const detail = await detailResponse.json();
    expect(detail.task.status).toBe('todo');
    expect(detail.messages.some((message: { body: string }) => message.body === 'Unblocked, continue.')).toBeTruthy();
  });

  test('danger zone factory reset clears the database', async ({ page }) => {
    // Create an agent to verify it gets deleted
    const testAgent = `Doomed Agent ${Date.now()}`;
    await page.goto('/agents');
    await page.getByTestId('agent-create-button').click();
    await page.locator('[data-modal="agent-quick-modal"]').getByPlaceholder('Nome').fill(testAgent);
    await page.locator('[data-modal="agent-quick-modal"] select[name="type"]').selectOption('noop');
    await page.getByTestId('agent-create-submit').click();
    await expect(page.getByRole('heading', { name: testAgent })).toBeVisible();

    // Navigate to settings and open reset modal
    await page.goto('/settings');
    await page.getByTestId('settings-reset-all').click();
    const modal = page.locator('[data-modal="reset-modal"]');
    await expect(modal).toBeVisible();

    // Fill confirm and submit
    await modal.getByLabel(/Please type/i).fill('RESET');
    await page.getByTestId('settings-reset-confirm').click();

    // Should redirect to dashboard and show empty or default state
    await expect(page).toHaveURL('/');

    // Verify agent is gone
    await page.goto('/agents');
    await expect(page.getByText(testAgent)).not.toBeVisible();
  });
});
