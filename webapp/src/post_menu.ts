// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {Client} from './client';
import {LANGUAGES} from './languages';
import {translationState} from './translation_state';

/**
 * Toggle translation for a post.
 * - If already showing translated text, flip back to original.
 * - Otherwise fetch the user's saved language preference (P2 fix), translate
 *   (or reuse the cached text if the lang matches), and mark as translated.
 */
export async function toggleTranslate(postId: string, client: Client): Promise<void> {
    const current = translationState.get(postId);

    if (current?.showing === 'translated') {
        translationState.toggle(postId);
        return;
    }

    // Determine target language: reuse previously chosen lang or fetch saved preference.
    const lang = current?.lang ?? (await client.getLang());

    // Reuse cached translation if we already have one for this lang.
    let text: string;
    if (current?.text && current.lang === lang) {
        text = current.text;
    } else {
        text = await client.translate(postId, lang);
    }

    translationState.set(postId, {showing: 'translated', lang, text});
}

/**
 * Persist a new default translation language for the current user.
 */
export async function setDefaultLang(code: string, client: Client): Promise<void> {
    await client.setLang(code);
}

/**
 * Register the post dropdown menu items with the Mattermost plugin registry.
 *
 * Two items are added:
 *  1. "Translate / Show original" — toggles translation for the clicked post.
 *  2. "Translate to" sub-menu — one entry per supported language that updates
 *     the user's saved language preference.
 *
 * NOTE: The flat `registerPostDropdownMenuAction` types its action as `() => void`
 * but at runtime Mattermost passes the postId as the first argument.  We therefore
 * declare the handler with a rest parameter to satisfy the type while still
 * reading the postId at runtime.
 */
export function registerPostMenu(registry: any, client: Client = new Client()): void {
    // Primary action: translate or flip back to original. This is the core
    // reachable path — register it independently so a missing sub-menu API
    // (older server versions) never blocks it.
    if (typeof registry?.registerPostDropdownMenuAction === 'function') {
        registry.registerPostDropdownMenuAction(
            'Translate / Show original',
            (...args: unknown[]) => {
                toggleTranslate(String(args[0]), client).catch(
                    (err: unknown) => console.error('[ai-translate] toggleTranslate failed', err), // eslint-disable-line no-console
                );
            },
            () => true,
        );
    }

    // Language sub-menu: lets the user change their saved preference. Best-effort
    // and guarded — the sub-menu registry API is not available on every server
    // version, and a failure here must not break plugin activation.
    if (typeof registry?.registerPostDropdownSubMenuAction !== 'function') {
        return;
    }
    try {
        const sub = registry.registerPostDropdownSubMenuAction({
            text: 'Translate to',
            action: () => {
                // intentionally empty — sub-items handle their own actions
            },
            filter: () => true,
        });

        if (!sub || typeof sub.rootRegisterMenuItem !== 'function') {
            return;
        }

        LANGUAGES.forEach((l) => {
            sub.rootRegisterMenuItem(
                l.name,
                () => {
                    setDefaultLang(l.code, client).catch(
                        (err: unknown) => console.error('[ai-translate] setDefaultLang failed', err), // eslint-disable-line no-console
                    );
                },
                () => true,
            );
        });
    } catch (err) {
        console.error('[ai-translate] failed to register language sub-menu', err); // eslint-disable-line no-console
    }
}
