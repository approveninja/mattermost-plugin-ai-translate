// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React, {useEffect, useReducer, useState} from 'react';

import {Client} from '../client';
import {LANGUAGES, DEFAULT_LANGUAGE} from '../languages';
import {translationState} from '../translation_state';

const client = new Client();

export function TranslateControl({postId}: {postId: string}) {
    const [, forceUpdate] = useReducer((x: number) => x + 1, 0);
    useEffect(() => translationState.subscribe(forceUpdate), [postId]);

    const existing = translationState.get(postId);
    const [lang, setLang] = useState(existing?.lang || DEFAULT_LANGUAGE);
    const [busy, setBusy] = useState(false);
    const [menuOpen, setMenuOpen] = useState(false);

    const showing = translationState.get(postId)?.showing || 'original';

    const doTranslate = async (targetLang: string) => {
        setBusy(true);
        try {
            const cached = translationState.get(postId);
            let text = cached?.text;
            if (!text || cached?.lang !== targetLang) {
                text = await client.translate(postId, targetLang);
                await client.setLang(targetLang);
            }
            translationState.set(postId, {showing: 'translated', lang: targetLang, text});
            setLang(targetLang);
        } finally {
            setBusy(false);
        }
    };

    const onMainClick = () => {
        if (showing === 'translated') {
            translationState.toggle(postId);
        } else {
            doTranslate(lang);
        }
    };

    const label = showing === 'translated' ? '↩ original' : `→ ${lang}`;

    return (
        <span className='ai-translate-control'>
            <button
                disabled={busy}
                onClick={onMainClick}
            >{busy ? '…' : label}</button>
            <button onClick={() => setMenuOpen((o) => !o)}>{'▾'}</button>
            {menuOpen && (
                <ul className='ai-translate-menu'>
                    {LANGUAGES.map((l) => (
                        <li key={l.code}>
                            <button
                                onClick={() => {
                                    setMenuOpen(false);
                                    doTranslate(l.code);
                                }}
                            >{l.name}</button>
                        </li>
                    ))}
                </ul>
            )}
        </span>
    );
}
