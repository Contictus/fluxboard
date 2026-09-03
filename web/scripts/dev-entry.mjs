import { spawn } from 'node:child_process';

const child = spawn('pnpm', ['dev'], { stdio: 'inherit' });
const routes = ['/', '/app', '/account/profile', '/account/notifications', '/account/sessions'];

for (let attempt = 0; attempt < 60; attempt += 1) {
  try {
    const response = await fetch('http://127.0.0.1:3000/');
    if (response.ok) break;
  } catch {
    // Next is still starting.
  }
  await new Promise((resolve) => setTimeout(resolve, 500));
}

await Promise.all(
  routes.map(async (route) => {
    try {
      await fetch(`http://127.0.0.1:3000${route}`);
    } catch {
      // A failed warm-up must not hide the dev server process.
    }
  }),
);

const forward = (signal) => child.kill(signal);
process.on('SIGTERM', () => forward('SIGTERM'));
process.on('SIGINT', () => forward('SIGINT'));

child.on('exit', (code, signal) => {
  process.exit(code ?? (signal ? 1 : 0));
});
