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
 * Translate a specific post into an explicit language, show it, and persist
 * the choice as the user's default.
 */
export async function translatePostTo(postId: string, lang: string, client: Client): Promise<void> {
    const text = await client.translate(postId, lang);
    translationState.set(postId, {showing: 'translated', lang, text});
    await client.setLang(lang);
}

/**
 * Register the post dropdown menu items with the Mattermost plugin registry.
 *
 * Two items are added:
 *  1. "Translate / Show original" — toggles translation for the clicked post.
 *  2. "Translate to" sub-menu — one entry per supported language that translates
 *     the clicked post into that language and persists the choice as the default.
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

    // Language sub-menu: lets the user translate the clicked post into a specific
    // language and persist that choice as their default. Best-effort and guarded
    // — the sub-menu registry API is not available on every server version, and a
    // failure here must not break plugin activation.
    if (typeof registry?.registerPostDropdownSubMenuAction !== 'function') {
        return;
    }
    try {
        // Capture the postId from the root filter so sub-item actions can use it.
        let currentPostId = '';
        const sub = registry.registerPostDropdownSubMenuAction({
            text: 'Translate to',
            action: () => { /* root click just opens the submenu */ },
            filter: (postId: string) => {
                currentPostId = String(postId);
                return true;
            },
        });

        if (!sub || typeof sub.rootRegisterMenuItem !== 'function') {
            return;
        }

        LANGUAGES.forEach((l) => {
            sub.rootRegisterMenuItem(
                l.name,
                () => {
                    translatePostTo(currentPostId, l.code, client).catch(
                        (err: unknown) => console.error('[ai-translate] translatePostTo failed', err), // eslint-disable-line no-console
                    );
                },
                () => true,
            );
        });
    } catch (err) {
        console.error('[ai-translate] failed to register language sub-menu', err); // eslint-disable-line no-console
    }
}
