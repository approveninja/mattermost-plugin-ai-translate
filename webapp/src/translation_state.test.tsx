// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {translationState} from './translation_state';

describe('translationState', () => {
    beforeEach(() => translationState.reset());

    describe('set / get round-trip', () => {
        it('returns undefined for an unknown post', () => {
            expect(translationState.get('unknown')).toBeUndefined();
        });

        it('returns the entry that was set', () => {
            translationState.set('p1', {showing: 'original', lang: 'EN'});
            expect(translationState.get('p1')).toEqual({showing: 'original', lang: 'EN'});
        });

        it('stores text when provided', () => {
            translationState.set('p2', {showing: 'translated', lang: 'DE', text: 'Hallo'});
            expect(translationState.get('p2')).toEqual({showing: 'translated', lang: 'DE', text: 'Hallo'});
        });
    });

    describe('toggle', () => {
        it('does nothing when post has no entry', () => {
            translationState.toggle('nonexistent');
            expect(translationState.get('nonexistent')).toBeUndefined();
        });

        it('flips showing from original to translated', () => {
            translationState.set('p3', {showing: 'original', lang: 'FR', text: 'Bonjour'});
            translationState.toggle('p3');
            expect(translationState.get('p3')?.showing).toBe('translated');
        });

        it('flips showing from translated to original', () => {
            translationState.set('p4', {showing: 'translated', lang: 'FR', text: 'Bonjour'});
            translationState.toggle('p4');
            expect(translationState.get('p4')?.showing).toBe('original');
        });

        it('can toggle back and forth', () => {
            translationState.set('p5', {showing: 'original', lang: 'ES', text: 'Hola'});
            translationState.toggle('p5');
            expect(translationState.get('p5')?.showing).toBe('translated');
            translationState.toggle('p5');
            expect(translationState.get('p5')?.showing).toBe('original');
        });
    });

    describe('displayText', () => {
        it('returns the original when no entry exists', () => {
            expect(translationState.displayText('nope', 'hello')).toBe('hello');
        });

        it('returns the original when showing is original', () => {
            translationState.set('p6', {showing: 'original', lang: 'EN', text: 'Hello'});
            expect(translationState.displayText('p6', 'source text')).toBe('source text');
        });

        it('returns the original when showing is translated but text is absent', () => {
            translationState.set('p7', {showing: 'translated', lang: 'EN'});
            expect(translationState.displayText('p7', 'source text')).toBe('source text');
        });

        it('returns the translated text when showing is translated and text is present', () => {
            translationState.set('p8', {showing: 'translated', lang: 'UA', text: 'Привіт'});
            expect(translationState.displayText('p8', 'Hello')).toBe('Привіт');
        });
    });

    describe('reset', () => {
        it('clears all entries', () => {
            translationState.set('p9', {showing: 'original', lang: 'EN'});
            translationState.set('p10', {showing: 'translated', lang: 'RU', text: 'Привет'});
            translationState.reset();
            expect(translationState.get('p9')).toBeUndefined();
            expect(translationState.get('p10')).toBeUndefined();
        });
    });

    describe('subscribe', () => {
        it('calls the subscriber on set', () => {
            const cb = jest.fn();
            const unsubscribe = translationState.subscribe(cb);
            translationState.set('p11', {showing: 'original', lang: 'EN'});
            expect(cb).toHaveBeenCalledTimes(1);
            unsubscribe();
        });

        it('calls the subscriber on toggle', () => {
            const cb = jest.fn();
            translationState.set('p12', {showing: 'original', lang: 'EN'});
            const unsubscribe = translationState.subscribe(cb);
            translationState.toggle('p12');
            expect(cb).toHaveBeenCalledTimes(1);
            unsubscribe();
        });

        it('does not call subscriber after unsubscribe', () => {
            const cb = jest.fn();
            const unsubscribe = translationState.subscribe(cb);
            unsubscribe();
            translationState.set('p13', {showing: 'original', lang: 'EN'});
            expect(cb).not.toHaveBeenCalled();
        });
    });
});
