import { shell } from 'electron';

const LOCAL_URL = /^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?(?:\/|$)/i;
const EXTERNAL_HOSTS = [
  'github.com',
  'raw.githubusercontent.com',
  'objects.githubusercontent.com',
  'docs.github.com',
];

export function isLocalUrl(url: string): boolean {
  return LOCAL_URL.test(url);
}

export function isAllowedExternal(url: string): boolean {
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== 'https:') return false;
    const host = parsed.hostname.toLowerCase();
    return EXTERNAL_HOSTS.some((allowed) => host === allowed || host.endsWith(`.${allowed}`));
  } catch {
    return false;
  }
}

export async function openExternal(url: string): Promise<boolean> {
  if (!isAllowedExternal(url)) return false;
  await shell.openExternal(url);
  return true;
}
