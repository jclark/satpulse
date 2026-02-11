import {h} from 'preact';
import {useCallback} from 'preact/hooks';
import {LoadMsgFile, SendMsgFile, CancelMsgSend} from '../wailsjs/go/main/App';
import type {ConnState, MsgFileTag, SendLine} from './app';

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

const btnClass = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100';
const btnPrimary = 'px-3.5 py-1 rounded text-xs border border-blue-600 bg-blue-600 text-white cursor-pointer hover:bg-blue-700 disabled:opacity-50 disabled:cursor-default';
const btnDanger = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-red-600 hover:border-red-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100';

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

export function MsgFilePanel({connState, msgFilePath, setMsgFilePath, msgFileTags, setMsgFileTags, selectedTagIndex, setSelectedTagIndex, sendLines, setSendLines, sendState, setSendState, addToast}: Props) {
    const handleOpen = useCallback(async () => {
        try {
            const info = await LoadMsgFile();
            if (!info) return; // user cancelled
            setMsgFilePath(info.path);
            setMsgFileTags(info.tags || []);
            setSendLines([]);
            setSendState('idle');
            const tags = info.tags || [];
            // Auto-select: if exactly one tag, or if a default (empty) tag exists
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
        <div class="flex flex-col h-full">
            {/* File selection */}
            <div class="flex items-center gap-3 px-4 pt-4 pb-2 shrink-0">
                <button class={btnClass} onClick={handleOpen}>Open...</button>
                <span class="text-xs text-gray-600 dark:text-gray-300">{msgFilePath || ''}</span>
            </div>

            {/* Tag selector (scrollable) */}
            <div class="overflow-y-auto px-4 flex-2">
                <table class="w-full text-xs border-collapse">
                    <thead>
                        <tr class="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700 sticky top-0 bg-white dark:bg-gray-800">
                            <th class="py-1.5 px-2 font-medium w-36">Tag</th>
                            <th class="py-1.5 px-2 font-medium">Description</th>
                            <th class="py-1.5 px-2 font-medium text-right w-20"># Msgs</th>
                        </tr>
                    </thead>
                    <tbody>
                        {msgFileTags.map((t, i) => (
                            <tr
                                key={i}
                                class={`cursor-pointer border-b border-gray-100 dark:border-gray-800 ${
                                    i === selectedTagIndex
                                        ? 'bg-blue-50 dark:bg-blue-950'
                                        : 'hover:bg-gray-50 dark:hover:bg-gray-900'
                                }`}
                                onClick={() => setSelectedTagIndex(i)}
                            >
                                <td class="py-1.5 px-2">
                                    {t.tag === '' ? <em class="text-gray-400">(default)</em> : t.tag}
                                </td>
                                <td class="py-1.5 px-2 text-gray-500 dark:text-gray-400">{t.desc || ''}</td>
                                <td class="py-1.5 px-2 text-right">{t.msgCount}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>

            {/* Send / Cancel buttons */}
            <div class="flex items-center gap-2 px-4 py-2 shrink-0">
                <button class={btnPrimary} disabled={sendDisabled} onClick={handleSend}>
                    Send
                </button>
                <button class={btnDanger} disabled={!cancelEnabled} onClick={handleCancel}>
                    Cancel
                </button>
            </div>

            {/* Send/response display */}
            <div class="flex gap-4 px-4 pb-4 flex-1 min-h-[4em]">
                {/* Left side: send progress */}
                <div class="flex-1 font-mono text-xs leading-relaxed text-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-700 rounded p-2 overflow-y-auto bg-white dark:bg-gray-900">
                    {sendLines.map((line, i) => (
                        <div key={i} class={line.status === 'error' ? 'text-red-500' : ''}>
                            {formatSendLine(line)}
                        </div>
                    ))}
                </div>
                {/* Right side: receiver responses (phase 7b) */}
                <div class="flex-1 font-mono text-xs leading-relaxed text-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-700 rounded p-2 overflow-y-auto bg-white dark:bg-gray-900">
                </div>
            </div>
        </div>
    );
}
