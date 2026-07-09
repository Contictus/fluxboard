'use client';

import { useRef, useState } from 'react';

import { useOrgMembers } from '@/lib/org/use-members';
import { cn } from '@/lib/utils';

// A comment textarea with @mention autocomplete over the org's members
// (FR-TASK-005). Typing "@" opens a filtered popup; selecting inserts
// "@Display Name ". The stored body keeps the plain "@name" text — the backend
// treats the body as markdown, so no special mention encoding is required here.
export function MentionInput({
  value,
  onChange,
  placeholder,
  disabled,
  rows = 3,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  disabled?: boolean;
  rows?: number;
}) {
  const { members } = useOrgMembers();
  const ref = useRef<HTMLTextAreaElement>(null);
  const [query, setQuery] = useState<string | null>(null);
  const [active, setActive] = useState(0);

  // Extract the @token immediately before the caret, if any.
  function tokenAt(text: string, caret: number): { start: number; term: string } | null {
    const upto = text.slice(0, caret);
    const at = upto.lastIndexOf('@');
    if (at < 0) return null;
    // The char before "@" must be start-of-line or whitespace.
    if (at > 0 && !/\s/.test(upto[at - 1] as string)) return null;
    const term = upto.slice(at + 1);
    if (/\s/.test(term)) return null; // mention token can't contain whitespace
    return { start: at, term };
  }

  const suggestions =
    query === null
      ? []
      : members
          .filter((m) => {
            const q = query.toLowerCase();
            return m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q);
          })
          .slice(0, 6);

  function handleChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const text = e.target.value;
    onChange(text);
    const tok = tokenAt(text, e.target.selectionStart ?? text.length);
    setQuery(tok ? tok.term : null);
    setActive(0);
  }

  function insert(name: string) {
    const el = ref.current;
    if (!el) return;
    const caret = el.selectionStart ?? value.length;
    const tok = tokenAt(value, caret);
    if (!tok) return;
    const before = value.slice(0, tok.start);
    const after = value.slice(caret);
    const mention = `@${name} `;
    const next = before + mention + after;
    onChange(next);
    setQuery(null);
    // Restore caret just after the inserted mention.
    requestAnimationFrame(() => {
      const pos = (before + mention).length;
      el.focus();
      el.setSelectionRange(pos, pos);
    });
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (query === null || suggestions.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive((i) => (i + 1) % suggestions.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive((i) => (i - 1 + suggestions.length) % suggestions.length);
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault();
      const pick = suggestions[active];
      if (pick) insert(pick.name || pick.email);
    } else if (e.key === 'Escape') {
      setQuery(null);
    }
  }

  return (
    <div className="relative">
      <textarea
        ref={ref}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onBlur={() => setTimeout(() => setQuery(null), 150)}
        placeholder={placeholder}
        disabled={disabled}
        rows={rows}
        className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
      />
      {query !== null && suggestions.length > 0 ? (
        <ul className="absolute z-20 mt-1 w-64 overflow-hidden rounded-md border bg-card shadow-md">
          {suggestions.map((m, i) => (
            <li key={m.user_id}>
              <button
                type="button"
                onMouseDown={(e) => {
                  e.preventDefault();
                  insert(m.name || m.email);
                }}
                className={cn(
                  'flex w-full flex-col items-start px-3 py-1.5 text-left text-sm',
                  i === active ? 'bg-secondary' : 'hover:bg-secondary/60',
                )}
              >
                <span className="font-medium">{m.name || m.email}</span>
                {m.name ? <span className="text-xs text-muted-foreground">{m.email}</span> : null}
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
