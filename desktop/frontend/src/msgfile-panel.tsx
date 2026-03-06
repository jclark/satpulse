import {h, Fragment} from 'preact';
import {useCallback, useEffect, useRef} from 'preact/hooks';
import {LoadMsgFile, SendMsgFile, CancelMsgSend, DecodePacket} from '../wailsjs/go/main/App';
import type {ConnState, MsgFileTag, SendLine, ResponseLine} from './app';
import {Button, Card} from './ui';
import {useState} from 'preact/hooks';

interface Props {
    connState: ConnState;
    msgFilePath: string;
    setMsgFilePath: (p: string) => void;
    msgFileTags: MsgFileTag[];
    setMsgFileTags: (tags: MsgFileTag[]) => void;
    selectedTagIndex: number;
    setSelectedTagIndex: (i: number) => void;
    activeTagIndex: number;
    setActiveTagIndex: (i: number) => void;
    tagArmed: boolean;
    setTagArmed: (armed: boolean) => void;
    sendLines: SendLine[];
    setSendLines: (lines: SendLine[]) => void;
    responseLines: ResponseLine[];
    setResponseLines: (lines: ResponseLine[]) => void;
    selectedResponseIndex: number;
    setSelectedResponseIndex: (i: number) => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
}

interface StatusEntry {
    type: 'send' | 'response';
    sendLine?: SendLine;
    response?: ResponseLine;
    responseIdx?: number;
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

function formatResponseLabel(r: ResponseLine): string {
    if (r.kind === 'ack') {
        const prefix = (r.msgCount ?? 0) > 1 ? `Message ${r.responseTo + 1}` : 'Message';
        if (!r.ackError) return `${prefix} accepted`;
        return `${prefix} rejected: ${r.ackError}`;
    }
    if (r.tag && r.msgID) return `Received ${r.tag}-${r.msgID}`;
    if (r.tag) return `Received ${r.tag}`;
    if (r.text) {
        const preview = r.text.length > 40 ? r.text.slice(0, 40) + '...' : r.text;
        return preview;
    }
    return 'Received response';
}

function isClickable(r: ResponseLine): boolean {
    return r.kind === 'other' || r.kind === 'maybe';
}

// Build an interleaved list of send lines and response lines in chronological order.
// Send lines are ordered by index. Response lines are inserted after the send line
// they relate to (by responseTo), or at the end if unrelated.
function buildStatusEntries(sendLines: SendLine[], responseLines: ResponseLine[]): StatusEntry[] {
    const entries: StatusEntry[] = [];
    // Track which responses have been placed.
    const placed = new Set<number>();
    for (const sl of sendLines) {
        entries.push({type: 'send', sendLine: sl});
        // Insert responses related to this send line (0-based index = sl.index - 1).
        const sentIdx = sl.index - 1;
        for (let ri = 0; ri < responseLines.length; ri++) {
            if (placed.has(ri)) continue;
            const r = responseLines[ri];
            if (r.responseTo === sentIdx) {
                entries.push({type: 'response', response: r, responseIdx: ri});
                placed.add(ri);
            }
        }
    }
    // Append unplaced responses (other/maybe not tied to a specific send).
    for (let ri = 0; ri < responseLines.length; ri++) {
        if (!placed.has(ri)) {
            entries.push({type: 'response', response: responseLines[ri], responseIdx: ri});
        }
    }
    return entries;
}

export function MsgFilePanel({
    connState,
    msgFilePath,
    setMsgFilePath,
    msgFileTags,
    setMsgFileTags,
    selectedTagIndex,
    setSelectedTagIndex,
    activeTagIndex,
    setActiveTagIndex,
    tagArmed,
    setTagArmed,
    sendLines,
    setSendLines,
    responseLines,
    setResponseLines,
    selectedResponseIndex,
    setSelectedResponseIndex,
    addToast,
}: Props) {
    const [decodeResult, setDecodeResult] = useState<string | null>(null);
    const leftPaneRef = useRef<HTMLDivElement>(null);

    // Auto-scroll left pane when new entries appear.
    useEffect(() => {
        const el = leftPaneRef.current;
        if (el) el.scrollTop = el.scrollHeight;
    }, [sendLines, responseLines]);

    const handleOpen = useCallback(async () => {
        try {
            const info = await LoadMsgFile();
            if (!info) return;
            setMsgFilePath(info.path);
            setMsgFileTags(info.tags || []);
            setSendLines([]);
            setResponseLines([]);
            setSelectedResponseIndex(-1);
            setActiveTagIndex(-1);
            setTagArmed(false);
            setDecodeResult(null);
            const tags = info.tags || [];
            if (tags.length === 1) {
                setSelectedTagIndex(0);
                setTagArmed(true);
            } else if (tags.length > 0 && tags[0].tag === '') {
                setSelectedTagIndex(0);
                setTagArmed(true);
            } else {
                setSelectedTagIndex(-1);
            }
        } catch (e: any) {
            addToast(e.message || 'Failed to load file', 'error');
        }
    }, [setMsgFilePath, setMsgFileTags, setSendLines, setSelectedTagIndex, setResponseLines, setSelectedResponseIndex, setActiveTagIndex, setTagArmed, addToast]);

    const handleTagClick = useCallback(async (i: number) => {
        // Cancel pausing if active.
        if (connState === 'pausing') {
            CancelMsgSend().catch(() => {});
        }
        // Clear previous results.
        setSendLines([]);
        setResponseLines([]);
        setSelectedResponseIndex(-1);
        setActiveTagIndex(-1);
        setDecodeResult(null);
        // Select this row (armed for send).
        setSelectedTagIndex(i);
        setTagArmed(true);
    }, [connState, setSendLines, setResponseLines, setSelectedResponseIndex, setActiveTagIndex, setSelectedTagIndex, setTagArmed]);

    const handleSend = useCallback(async () => {
        if (selectedTagIndex < 0 || selectedTagIndex >= msgFileTags.length) return;
        const tag = msgFileTags[selectedTagIndex].tag;
        // Transition: selected -> has-results.
        setActiveTagIndex(selectedTagIndex);
        setTagArmed(false);
        setSendLines([]);
        setResponseLines([]);
        setSelectedResponseIndex(-1);
        setDecodeResult(null);
        try {
            await SendMsgFile(tag);
        } catch (e: any) {
            addToast(e.message || 'Send failed', 'error');
        }
    }, [selectedTagIndex, msgFileTags, setSendLines, setResponseLines, setSelectedResponseIndex, setActiveTagIndex, setTagArmed, addToast]);

    // Decode selected response.
    useEffect(() => {
        if (selectedResponseIndex < 0 || selectedResponseIndex >= responseLines.length) {
            setDecodeResult(null);
            return;
        }
        const r = responseLines[selectedResponseIndex];
        if (r.bin) {
            let cancelled = false;
            DecodePacket(r.bin, false).then(result => {
                if (cancelled) return;
                setDecodeResult(result ? JSON.stringify(result, null, 2) : null);
            }).catch(() => { if (!cancelled) setDecodeResult(null); });
            return () => { cancelled = true; };
        }
        setDecodeResult(null);
    }, [selectedResponseIndex, responseLines]);

    const handleResponseClick = useCallback((idx: number) => {
        if (!isClickable(responseLines[idx])) return;
        setSelectedResponseIndex(idx);
    }, [responseLines, selectedResponseIndex, setSelectedResponseIndex]);

    const sendEnabled = connState === 'connected' && tagArmed && selectedTagIndex >= 0 && !!msgFilePath;
    const hasResults = activeTagIndex >= 0;
    const selectedResponse = selectedResponseIndex >= 0 && selectedResponseIndex < responseLines.length
        ? responseLines[selectedResponseIndex] : null;
    const statusEntries = buildStatusEntries(sendLines, responseLines);

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
                        {msgFileTags.map((t, i) => {
                            const isSelected = i === selectedTagIndex && tagArmed;
                            const isDotted = i === activeTagIndex && !tagArmed;
                            return (
                                <tr
                                    key={i}
                                    class={`cursor-pointer border-b border-border-subtle ${
                                        isSelected ? 'bg-surface-3' : 'hover:bg-surface-1'
                                    }`}
                                    onClick={() => handleTagClick(i)}
                                >
                                    <td class="px-2 py-1.5 tabular-nums">
                                        {t.tag === '' ? <em class="text-text-muted">(default)</em> : t.tag}
                                        {isDotted && <span class="ml-1.5 text-sm leading-none text-accent">&#x2022;</span>}
                                    </td>
                                    <td class="px-2 py-1.5 text-text-secondary">{t.desc || ''}</td>
                                    <td class="px-2 py-1.5 text-right">{t.msgCount}</td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </div>

            <div class="flex shrink-0 items-center gap-2 px-4 py-2">
                <Button variant="primary" disabled={!sendEnabled} onClick={handleSend}>
                    Send
                </Button>
            </div>

            {/* Two-pane area: left status, right raw+decode */}
            <div class="flex min-h-[6em] flex-1 gap-2 px-4 pb-4">
                {/* Left pane: interleaved send/response status */}
                <Card class="w-56 shrink-0 overflow-y-auto p-2 text-xs leading-relaxed" ref={leftPaneRef}>
                    {hasResults && statusEntries.map((entry, i) => {
                        if (entry.type === 'send' && entry.sendLine) {
                            return (
                                <div key={`s${i}`} class={`text-text-muted ${entry.sendLine.status === 'error' ? 'text-danger' : ''}`}>
                                    {formatSendLine(entry.sendLine)}
                                </div>
                            );
                        }
                        if (entry.type === 'response' && entry.response) {
                            const r = entry.response;
                            const ridx = entry.responseIdx!;
                            const clickable = isClickable(r);
                            const selected = ridx === selectedResponseIndex;
                            let cls = '';
                            if (r.kind === 'ack' && !r.ackError) cls = 'text-success';
                            else if (r.kind === 'ack' && r.ackError) cls = 'text-danger';
                            else if (r.kind === 'maybe') cls = 'text-text-muted';
                            else cls = 'text-text-primary';
                            if (clickable) cls += ' cursor-pointer hover:bg-surface-3';
                            if (selected) cls += ' bg-surface-3';
                            return (
                                <div
                                    key={`r${i}`}
                                    class={cls}
                                    onClick={clickable ? () => handleResponseClick(ridx) : undefined}
                                >
                                    {formatResponseLabel(r)}
                                </div>
                            );
                        }
                        return null;
                    })}
                    {connState === 'pausing' && (
                        <div class="text-text-muted italic">Listening...</div>
                    )}
                    {!hasResults && !tagArmed && (
                        <div class="text-text-muted">Select a tag and click Send</div>
                    )}
                </Card>

                {/* Right pane: raw content + decode */}
                <Card class="flex flex-1 flex-col overflow-hidden p-2">
                    {selectedResponse ? (
                        <>
                            <div class="shrink-0 overflow-x-auto border-b border-border-subtle pb-1 mb-1 font-mono text-xs text-text-primary whitespace-pre break-all">
                                {selectedResponse.bin || selectedResponse.text || ''}
                            </div>
                            <div class="flex-1 overflow-y-auto font-mono text-xs text-text-primary whitespace-pre-wrap">
                                {decodeResult || ''}
                            </div>
                        </>
                    ) : hasResults && responseLines.some(isClickable) ? (
                        <div class="flex h-full items-center justify-center text-xs text-text-muted">
                            Click a response to view
                        </div>
                    ) : null}
                </Card>
            </div>
        </div>
    );
}
