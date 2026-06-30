// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';
import type {Store} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import type {PluginRegistry} from 'types/mattermost-webapp';

import {TranslateControl} from './components/translate_control';
import {translationState} from './translation_state';

// TranslateControl is imported for future per-post mounting. Per-post mounting
// via registry is pending the Task 0 webapp spike (plan Task 0 Step 3).
// eslint-disable-next-line @typescript-eslint/no-unused-vars
void TranslateControl;

export default class Plugin {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        // @see https://developers.mattermost.com/extend/plugins/webapp/reference/

        // Swap displayed post text with the cached translation when active.
        registry.registerMessageWillFormatHook((post: any, message: string) =>
            translationState.displayText(post.id, message));

        // TODO (Task 0 Step 3 spike): register TranslateControl as a per-post
        // action component once the correct registry mechanism is confirmed.
        // Candidate: registry.registerPostActionComponent(TranslateControl)
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
