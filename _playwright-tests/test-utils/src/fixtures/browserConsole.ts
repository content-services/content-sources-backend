import { test as base, type ConsoleMessage } from '@playwright/test';
import fs from 'fs/promises';

export type CapturedBrowserMessage = {
  source: 'console' | 'pageerror';
  level: string;
  text: string;
  location?: string;
  timestamp: string;
};

const ATTENTION_LEVELS = new Set(['error', 'warning', 'pageerror']);

function formatLocation(msg: ConsoleMessage): string | undefined {
  const loc = msg.location();
  if (!loc.url) {
    return undefined;
  }

  return `${loc.url}:${loc.lineNumber}:${loc.columnNumber}`;
}

function formatMessagesText(messages: CapturedBrowserMessage[]): string {
  return messages
    .map((message) => {
      const location = message.location ? ` (${message.location})` : '';
      return `[${message.timestamp}] [${message.level}]${location} ${message.text}`;
    })
    .join('\n');
}

function shouldAttachMessages(
  messages: CapturedBrowserMessage[],
  status: string | undefined,
): boolean {
  if (messages.length === 0) {
    return false;
  }

  if (status !== 'passed' && status !== 'skipped') {
    return true;
  }

  return messages.some((message) => ATTENTION_LEVELS.has(message.level));
}

/**
 * Captures browser console output and uncaught page errors for each test.
 * Enabled when CAPTURE_BROWSER_CONSOLE=true (diagnostic runs only).
 * Attaches readable and JSON artifacts when a test fails or emits errors or warnings.
 */
export const browserConsoleTest = base.extend({
  page: async ({ page }, use, testInfo) => {
    const messages: CapturedBrowserMessage[] = [];

    page.on('console', (msg) => {
      messages.push({
        source: 'console',
        level: msg.type(),
        text: msg.text(),
        location: formatLocation(msg),
        timestamp: new Date().toISOString(),
      });
    });

    page.on('pageerror', (error) => {
      messages.push({
        source: 'pageerror',
        level: 'pageerror',
        text: [error.message, error.stack].filter(Boolean).join('\n'),
        timestamp: new Date().toISOString(),
      });
    });

    await use(page);

    if (!shouldAttachMessages(messages, testInfo.status)) {
      return;
    }

    const textBody = formatMessagesText(messages);
    const jsonBody = JSON.stringify(messages, null, 2);

    await testInfo.attach('browser-console.txt', {
      body: textBody,
      contentType: 'text/plain',
    });

    await testInfo.attach('browser-console.json', {
      body: jsonBody,
      contentType: 'application/json',
    });

    await fs.writeFile(testInfo.outputPath('browser-console.txt'), textBody);
    await fs.writeFile(testInfo.outputPath('browser-console.json'), jsonBody);
  },
});
