// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';
import type {Store} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import type {PluginRegistry} from 'types/mattermost-webapp';

import {registerPostMenu} from './post_menu';
import {translationState} from './translation_state';

export default class Plugin {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        // @see https://developers.mattermost.com/extend/plugins/webapp/reference/

        // Swap displayed post text with the cached translation when active.
        registry.registerMessageWillFormatHook((post: any, message: string) =>
            translationState.displayText(post.id, message));

        // Register post dropdown menu items: "Translate / Show original" and
        // the "Translate to" language sub-menu.  This is the primary UI entry
        // point that makes the translate feature reachable (P1 fix).
        registerPostMenu(registry);
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
