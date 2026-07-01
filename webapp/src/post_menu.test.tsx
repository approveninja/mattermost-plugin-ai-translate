// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {Client} from './client';
import {toggleTranslate, translatePostTo, registerPostMenu} from './post_menu';
import {translationState} from './translation_state';

describe('post_menu', () => {
    let fakeClient: jest.Mocked<Client>;

    beforeEach(() => {
        translationState.reset();
        fakeClient = {
            translate: jest.fn().mockResolvedValue('Hello'),
            getLang: jest.fn().mockResolvedValue('RU'),
            setLang: jest.fn().mockResolvedValue(undefined),
        } as unknown as jest.Mocked<Client>;
    });

    describe('toggleTranslate', () => {
        it('calls getLang and translate when no prior state, stores result', async () => {
            await toggleTranslate('p1', fakeClient);

            expect(fakeClient.getLang).toHaveBeenCalledTimes(1);
            expect(fakeClient.translate).toHaveBeenCalledWith('p1', 'RU');
            expect(translationState.get('p1')).toEqual({
                showing: 'translated',
                lang: 'RU',
                text: 'Hello',
            });
        });

        it('flips to original and does NOT call translate a second time when already translated', async () => {
            await toggleTranslate('p1', fakeClient);

            // reset mock call counts but keep resolved values
            fakeClient.translate.mockClear();
            fakeClient.getLang.mockClear();

            await toggleTranslate('p1', fakeClient);

            expect(fakeClient.translate).not.toHaveBeenCalled();
            expect(translationState.get('p1')?.showing).toBe('original');
        });
    });

    describe('translatePostTo', () => {
        it('translates the post, updates state, and persists the language', async () => {
            await translatePostTo('p2', 'FR', fakeClient);

            expect(fakeClient.translate).toHaveBeenCalledWith('p2', 'FR');
            expect(translationState.get('p2')).toEqual({
                showing: 'translated',
                lang: 'FR',
                text: 'Hello',
            });
            expect(fakeClient.setLang).toHaveBeenCalledWith('FR');
        });
    });

    describe('registerPostMenu wiring', () => {
        it('sub-item action translates the post captured by the root filter', async () => {
            let rootFilter: ((postId: string) => boolean) | undefined;
            const subItems: Array<() => void> = [];

            const fakeRegistry = {
                registerPostDropdownMenuAction: jest.fn(),
                registerPostDropdownSubMenuAction: jest.fn((opts: {text: string; action: () => void; filter: (postId: string) => boolean}) => {
                    rootFilter = opts.filter;
                    return {
                        rootRegisterMenuItem: (_text: string, action: () => void) => {
                            subItems.push(action);
                        },
                    };
                }),
            };

            registerPostMenu(fakeRegistry, fakeClient);

            // Simulate Mattermost calling the root filter with a postId.
            expect(rootFilter).toBeDefined();
            rootFilter!('p9');

            // Invoke the first sub-item (English).
            expect(subItems.length).toBeGreaterThan(0);
            subItems[0]();

            // Flush microtasks so the async translatePostTo resolves.
            await new Promise((r) => setTimeout(r, 0));
            await Promise.resolve();

            expect(fakeClient.translate).toHaveBeenCalledWith('p9', 'EN');
        });
    });
});
