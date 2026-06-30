const BASE = '/plugins/com.approveninja.ai-translate/api/v1';

export class Client {
    private async parse(res: Response): Promise<any> {
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            throw new Error(data.error || 'Request failed');
        }
        return data;
    }

    async translate(postId: string, targetLang: string): Promise<string> {
        const res = await fetch(`${BASE}/translate`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({postId, targetLang}),
        });
        const data = await this.parse(res);
        return data.translatedText as string;
    }

    async getLang(): Promise<string> {
        const res = await fetch(`${BASE}/prefs/lang`);
        const data = await this.parse(res);
        return data.lang as string;
    }

    async setLang(lang: string): Promise<void> {
        const res = await fetch(`${BASE}/prefs/lang`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({lang}),
        });
        await this.parse(res);
    }
}
