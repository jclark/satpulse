import {h} from 'preact';
import {useCallback} from 'preact/hooks';
import {LoadMsgFile, SendMsgFile, CancelMsgSend} from '../wailsjs/go/main/App';
import type {ConnState, MsgFileTag, SendLine} from './app';
import {Button, Card} from './ui';

interface Props {
    connState: ConnState;
    msgFilePath: string;
    setMsgFilePath: (p: string) => void;
    msgFileTags: MsgFileTag[];
    setMsgFileTags: (tags: MsgFileTag[]) => void;
    selectedTagIndex: number;
    setSelectedTagIndex: (i: number) => void;
    sendLines: SendLine[];
    setSendLines: (lines: SendLine[]) => void;
    sendState: 'idle' | 'sending' | 'done' | 'error';
    setSendState: (s: 'idle' | 'sending' | 'done' | 'error') => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
}

function formatSendLine(line: SendLine): string {
    const prefix = line.total > 1 ? `Sending message ${line.index}` : 'Sending message';
    switch (line.status) {
        case 'sending':
            return `${prefix}...`;
        case 'delaying':
            return `${prefix}...pausing...`;
        case 'done':
            return `${prefix}...done`;
        case 'error':
            return `${prefix}...${line.error || 'error'}`;
    }
}

export function MsgFilePanel({
    connState,
    msgFilePath,
    setMsgFilePath,
    msgFileTags,
    setMsgFileTags,
    selectedTagIndex,
    setSelectedTagIndex,
    sendLines,
    setSendLines,
    sendState,
    setSendState,
    addToast,
}: Props) {
    const handleOpen = useCallback(async () => {
        try {
            const info = await LoadMsgFile();
            if (!info) return;
            setMsgFilePath(info.path);
            setMsgFileTags(info.tags || []);
            setSendLines([]);
            setSendState('idle');
            const tags = info.tags || [];
            if (tags.length === 1) {
                setSelectedTagIndex(0);
            } else if (tags.length > 0 && tags[0].tag === '') {
                setSelectedTagIndex(0);
            } else {
                setSelectedTagIndex(-1);
            }
        } catch (e: any) {
            addToast(e.message || 'Failed to load file', 'error');
        }
    }, [setMsgFilePath, setMsgFileTags, setSendLines, setSendState, setSelectedTagIndex, addToast]);

    const handleSend = useCallback(async () => {
        if (selectedTagIndex < 0 || selectedTagIndex >= msgFileTags.length) return;
        const tag = msgFileTags[selectedTagIndex].tag;
        setSendLines([]);
        setSendState('sending');
        try {
            await SendMsgFile(tag);
        } catch (e: any) {
            setSendState('error');
            addToast(e.message || 'Send failed', 'error');
        }
    }, [selectedTagIndex, msgFileTags, setSendLines, setSendState, addToast]);

    const handleCancel = useCallback(async () => {
        try {
            await CancelMsgSend();
        } catch (e: any) {
            addToast(e.message || 'Cancel failed', 'error');
        }
    }, [addToast]);

    const sendDisabled = connState !== 'connected' || !msgFilePath || selectedTagIndex < 0;
    const cancelEnabled = connState === 'sending';

    return (
        <div class="flex h-full flex-col">
            <div class="flex shrink-0 items-center gap-3 px-4 pt-4 pb-2">
                <Button onClick={handleOpen}>Open...</Button>
                <span class="text-xs text-text-secondary">{msgFilePath || ''}</span>
            </div>

            <div class="mx-4 flex-2 overflow-y-auto rounded border border-border-subtle bg-surface-2">
                <table class="w-full border-collapse text-xs">
                    <thead class="text-text-secondary">
                        <tr class="sticky top-0 bg-surface-2">
                            <th class="w-36 px-2 py-1.5 text-left">Tag</th>
                            <th class="px-2 py-1.5 text-left">Description</th>
                            <th class="w-20 px-2 py-1.5 text-right"># Msgs</th>
                        </tr>
                    </thead>
                    <tbody>
                        {msgFileTags.map((t, i) => (
                            <tr
                                key={i}
                                class={`cursor-pointer border-b border-border-subtle ${
                                    i === selectedTagIndex ? 'bg-surface-3' : 'hover:bg-surface-1'
                                }`}
                                onClick={() => setSelectedTagIndex(i)}
                            >
                                <td class="px-2 py-1.5 tabular-nums">
                                    {t.tag === '' ? <em class="text-text-muted">(default)</em> : t.tag}
                                </td>
                                <td class="px-2 py-1.5 text-text-secondary">{t.desc || ''}</td>
                                <td class="px-2 py-1.5 text-right">{t.msgCount}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>

            <div class="flex shrink-0 items-center gap-2 px-4 py-2">
                <Button variant="primary" disabled={sendDisabled} onClick={handleSend}>
                    Send
                </Button>
                <Button variant="danger" disabled={!cancelEnabled} onClick={handleCancel}>
                    Cancel
                </Button>
            </div>

            <div class="flex min-h-[4em] flex-1 gap-4 px-4 pb-4">
                <Card class="flex-1 overflow-y-auto p-2 font-mono text-xs leading-relaxed text-text-primary">
                    {sendLines.map((line, i) => (
                        <div key={i} class={line.status === 'error' ? 'text-danger' : ''}>
                            {formatSendLine(line)}
                        </div>
                    ))}
                </Card>
                <Card class="flex-1 overflow-y-auto p-2 font-mono text-xs leading-relaxed text-text-primary" />
            </div>
        </div>
    );
}
