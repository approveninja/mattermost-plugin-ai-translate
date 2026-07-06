// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

type Entry = {showing: 'original' | 'translated'; lang: string; text?: string};

class TranslationState {
    private entries = new Map<string, Entry>();
    private subs = new Set<() => void>();

    get(postId: string): Entry | undefined {
        return this.entries.get(postId);
    }

    set(postId: string, entry: Entry) {
        this.entries.set(postId, entry);
        this.subs.forEach((cb) => cb());
    }

    toggle(postId: string) {
        const e = this.entries.get(postId);
        if (!e) {
            return;
        }
        e.showing = e.showing === 'translated' ? 'original' : 'translated';
        this.set(postId, e);
    }

    displayText(postId: string, original: string): string {
        const e = this.entries.get(postId);
        if (e && e.showing === 'translated' && e.text) {
            return e.text;
        }
        return original;
    }

    subscribe(cb: () => void): () => void {
        this.subs.add(cb);
        return () => this.subs.delete(cb);
    }

    reset() {
        this.entries.clear();
    }
}

export const translationState = new TranslationState();
