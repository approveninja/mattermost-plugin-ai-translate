import {Client} from './client';

describe('Client', () => {
    const base = '/plugins/com.approveninja.ai-translate/api/v1';

    afterEach(() => {
        (global.fetch as jest.Mock | undefined)?.mockReset?.();
    });

    it('posts to translate and returns text', async () => {
        global.fetch = jest.fn().mockResolvedValue({
            ok: true, json: async () => ({translatedText: 'Hello'}),
        }) as jest.Mock;
        const c = new Client();
        const out = await c.translate('post1', 'EN');
        expect(out).toBe('Hello');
        expect((global.fetch as jest.Mock).mock.calls[0][0]).toBe(`${base}/translate`);
    });

    it('throws the server error message', async () => {
        global.fetch = jest.fn().mockResolvedValue({
            ok: false, json: async () => ({error: 'Translation isn\'t set up yet.'}),
        }) as jest.Mock;
        const c = new Client();
        await expect(c.translate('p', 'EN')).rejects.toThrow('set up');
    });
});
