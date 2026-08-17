import { revalidatePath } from 'next/cache';
import { cookies } from 'next/headers';
import { currentLanguage, LANGUAGE_COOKIE, LANGUAGES } from '@/lib/api';

// Titles and names come back resolved for whatever Accept-Language the request
// carried, so switching language is a server round trip, not a client toggle.
//
// A form with a server action rather than a client component: it works without
// JavaScript, and there is no state to hold on the client — the next render
// simply asks the API in the other language.
export async function LanguageSwitch() {
  const active = await currentLanguage();

  async function choose(formData: FormData) {
    'use server';
    const code = String(formData.get('lang') ?? '');
    if (!LANGUAGES.some((l) => l.code === code)) return;

    const store = await cookies();
    store.set(LANGUAGE_COOKIE, code, {
      path: '/',
      maxAge: 60 * 60 * 24 * 365,
      sameSite: 'lax',
    });
    // Every page renders titles, so the whole tree is stale once this changes.
    revalidatePath('/', 'layout');
  }

  return (
    // The label belongs on the group. Putting aria-labelledby on each button
    // would override its own text, so every option would announce as "Title
    // language" and a screen reader user could not tell them apart.
    <form action={choose} aria-label="Title language" className="flex items-center gap-1">
      {LANGUAGES.map((l) => (
        <button
          key={l.code}
          type="submit"
          name="lang"
          value={l.code}
          aria-current={l.code === active ? 'true' : undefined}
          className={`rounded-full border px-3 py-1 text-sm transition-colors ${
            l.code === active
              ? 'border-fd-primary bg-fd-primary text-fd-primary-foreground'
              : 'border-fd-border hover:bg-fd-accent'
          }`}
        >
          {l.label}
        </button>
      ))}
    </form>
  );
}
