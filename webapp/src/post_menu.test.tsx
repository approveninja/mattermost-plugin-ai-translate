// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {Client} from './client';
import {toggleTranslate, setDefaultLang} from './post_menu';
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

    describe('setDefaultLang', () => {
        it('calls client.setLang with the given code', async () => {
            await setDefaultLang('FR', fakeClient);
            expect(fakeClient.setLang).toHaveBeenCalledWith('FR');
        });
    });
});
