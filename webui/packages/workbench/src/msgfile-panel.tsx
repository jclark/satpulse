import {h, Fragment} from 'preact';
import {useCallback, useEffect, useRef} from 'preact/hooks';
import {transport} from './transport';
import type {MsgFileEntry, MsgFileInfo} from './transport';
import type {ConnState, MsgFileTag, SendLine, ResponseLine} from './app';
import {Button, Card, Select, fieldLabelText, labeledControlText} from './ui';
import {useState} from 'preact/hooks';

interface Props {
    connState: ConnState;
    visible: boolean;
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
    clearRespSession: () => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
    defaultPort: string;
}

// ubxPorts lists the five u-blox ports a ubxvalport message can target.
// Upper-case to match the names the receiver reports on readback;
// ubxPortOffset matches case-insensitively when encoding.
const ubxPorts = ['I2C', 'UART1', 'UART2', 'USB', 'SPI'];

function formatSendStatus(sendLines: SendLine[], connState: ConnState): string {
    if (sendLines.length === 0) return '';
    const last = sendLines[sendLines.length - 1];
    if (last.status === 'error') return last.error || 'Error';
    // Hide once listening phase is over.
    if (connState === 'connected' && last.status === 'done' && last.index === last.total) return '';
    return 'Sending...' + '.'.repeat(Math.max(0, last.index - 1));
}

function formatResponseLine(r: ResponseLine): string {
    if (r.kind === 'ack') {
        const prefix = (r.msgCount ?? 0) > 1 ? `Message ${r.responseTo + 1}` : 'Message';
        if (!r.ackError) return `${prefix} accepted`;
        return `${prefix} rejected: ${r.ackError}`;
    }
    // other/maybe: show the raw content
    if (r.text) return r.text;
    const label = r.tag && r.msgID ? `${r.tag}-${r.msgID}` : r.tag || '';
    return label || (r.bin ? r.bin : 'Response');
}

function isClickable(r: ResponseLine): boolean {
    return r.kind === 'other' || r.kind === 'maybe';
}

export function MsgFilePanel({
    connState,
    visible,
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
    clearRespSession,
    addToast,
    defaultPort,
}: Props) {
    const [decodeResult, setDecodeResult] = useState<string | null>(null);
    const [selectedPort, setSelectedPort] = useState('');
    const [save, setSave] = useState(false);
    const leftPaneRef = useRef<HTMLDivElement>(null);

    // The web backend picks from the server-side library catalog (two
    // dropdowns plus Load); the desktop backend opens a native dialog.
    // The catalog is a flat name list in search order; sorting and
    // grouping by vendor happen here.
    const picker = !!(transport.msgFile?.listMsgFiles && transport.msgFile?.selectMsgFile);
    const [catalog, setCatalog] = useState<MsgFileEntry[]>([]);
    const [selectedVendor, setSelectedVendor] = useState('');
    const [selectedFile, setSelectedFile] = useState('');
    const vendors = [...new Set(catalog.map(e => e.vendor))].sort();
    const vendorFiles = catalog.filter(e => e.vendor === selectedVendor).map(e => e.file).sort();

    // Seed the port from the config-tab readback once it arrives, without
    // overriding a port the user has already chosen.
    useEffect(() => {
        if (defaultPort) setSelectedPort(p => p || defaultPort);
    }, [defaultPort]);

    // Auto-scroll left pane when new entries appear.
    useEffect(() => {
        const el = leftPaneRef.current;
        if (el) el.scrollTop = el.scrollHeight;
    }, [sendLines, responseLines]);

    // applyLoaded resets the send/response state for a freshly loaded file
    // and arms the sole/default tag, shared by the dialog and catalog paths.
    const applyLoaded = useCallback((info: MsgFileInfo) => {
        clearRespSession();
        setMsgFilePath(info.path);
        setMsgFileTags(info.tags || []);
        setSendLines([]);
        setResponseLines([]);
        setSelectedResponseIndex(-1);
        setActiveTagIndex(-1);
        setTagArmed(false);
        setDecodeResult(null);
        const tags = info.tags || [];
        if (tags.length === 1 || (tags.length > 0 && tags[0].tag === '')) {
            setSelectedTagIndex(0);
            setTagArmed(true);
        } else {
            setSelectedTagIndex(-1);
        }
    }, [clearRespSession, setMsgFilePath, setMsgFileTags, setSendLines, setSelectedTagIndex, setResponseLines, setSelectedResponseIndex, setActiveTagIndex, setTagArmed]);

    const handleOpen = useCallback(async () => {
        try {
            const info = await transport.msgFile!.loadMsgFile!();
            if (info) applyLoaded(info);
        } catch (e: any) {
            addToast(e.message || 'Failed to load file', 'error');
        }
    }, [applyLoaded, addToast]);

    // Refresh the catalog whenever the tab opens: it picks up files added
    // since the last visit, and the server's preselect (the session
    // vendor) applies only until the user has chosen a vendor.
    const loadCatalog = useCallback(async () => {
        try {
            const cat = await transport.msgFile!.listMsgFiles!();
            const names = cat.names || [];
            setCatalog(names);
            setSelectedVendor(prev => {
                if (prev && names.some(e => e.vendor === prev)) return prev;
                if (cat.preselect && names.some(e => e.vendor === cat.preselect)) return cat.preselect;
                return '';
            });
        } catch (e: any) {
            addToast(e.message || 'Failed to list message files', 'error');
        }
    }, [addToast]);

    useEffect(() => {
        if (picker && visible) loadCatalog();
    }, [picker, visible, loadCatalog]);

    // Keep the file selection valid as the vendor or catalog changes;
    // auto-pick when a vendor holds a single file.
    useEffect(() => {
        const files = catalog.filter(e => e.vendor === selectedVendor).map(e => e.file);
        setSelectedFile(prev => files.includes(prev) ? prev : (files.length === 1 ? files[0] : ''));
    }, [selectedVendor, catalog]);

    const handleLoad = useCallback(async () => {
        if (!selectedVendor || !selectedFile) return;
        try {
            applyLoaded(await transport.msgFile!.selectMsgFile!(selectedVendor, selectedFile));
        } catch (e: any) {
            addToast(e.message || 'Failed to load file', 'error');
        }
    }, [selectedVendor, selectedFile, applyLoaded, addToast]);

    const handleTagClick = useCallback(async (i: number) => {
        clearRespSession();
        // Clear previous results.
        setSendLines([]);
        setResponseLines([]);
        setSelectedResponseIndex(-1);
        setActiveTagIndex(-1);
        setDecodeResult(null);
        // Select this row (armed for send).
        setSelectedTagIndex(i);
        setTagArmed(true);
    }, [clearRespSession, setSendLines, setResponseLines, setSelectedResponseIndex, setActiveTagIndex, setSelectedTagIndex, setTagArmed]);

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
        const t = msgFileTags[selectedTagIndex];
        try {
            await transport.msgFile!.sendMsgFile(tag, t.needsPort ? selectedPort : '', !!t.saveAware && save);
        } catch (e: any) {
            addToast(e.message || 'Send failed', 'error');
        }
    }, [selectedTagIndex, msgFileTags, selectedPort, save, setSendLines, setResponseLines, setSelectedResponseIndex, setActiveTagIndex, setTagArmed, addToast]);

    // Decode selected response.
    useEffect(() => {
        if (selectedResponseIndex < 0 || selectedResponseIndex >= responseLines.length) {
            setDecodeResult(null);
            return;
        }
        const r = responseLines[selectedResponseIndex];
        const raw = r.bin || r.text;
        if (!raw) { setDecodeResult(null); return; }
        const hex = !!r.bin;
        let cancelled = false;
        transport.decodePacket(raw, {hex, out: false}).then(result => {
            if (cancelled) return;
            if (!result) { setDecodeResult(hex ? null : raw); return; }
            const keys = Object.keys(result);
            const display = keys.length === 1 && keys[0] === 'payload' ? result.payload : result;
            setDecodeResult(JSON.stringify(display, null, 2));
        }).catch(() => { if (!cancelled) setDecodeResult(hex ? null : raw); });
        return () => { cancelled = true; };
    }, [selectedResponseIndex, responseLines]);

    const handleResponseClick = useCallback((idx: number) => {
        if (!isClickable(responseLines[idx])) return;
        setSelectedResponseIndex(idx);
    }, [responseLines, selectedResponseIndex, setSelectedResponseIndex]);

    const selectedTag = selectedTagIndex >= 0 && selectedTagIndex < msgFileTags.length
        ? msgFileTags[selectedTagIndex] : undefined;
    const needsPort = !!selectedTag?.needsPort;
    const saveAware = !!selectedTag?.saveAware;
    // Show the port dropdown if we have a readback port or the loaded file
    // has any port-using tag; show save if the file has any save-aware tag.
    // Visibility tracks the loaded file/readback (not selection); selection
    // only controls greying, so the Send button never moves on selection.
    const showPort = !!defaultPort || msgFileTags.some(t => t.needsPort);
    const showSave = msgFileTags.some(t => t.saveAware);
    const sendEnabled = connState === 'connected' && tagArmed && selectedTagIndex >= 0 && !!msgFilePath
        && (!needsPort || selectedPort !== '');
    const hasResults = activeTagIndex >= 0;
    const selectedResponse = selectedResponseIndex >= 0 && selectedResponseIndex < responseLines.length
        ? responseLines[selectedResponseIndex] : null;
    const sendStatus = formatSendStatus(sendLines, connState);

    return (
        <div class="flex h-full flex-col">
            <div class="flex shrink-0 items-center gap-3 px-4 pt-4 pb-2">
                {picker ? (
                    <>
                        <label class={`flex items-center gap-1.5 ${fieldLabelText(false)}`}>
                            Vendor
                            <Select
                                class="w-40"
                                value={selectedVendor}
                                onChange={e => setSelectedVendor((e.target as HTMLSelectElement).value)}
                            >
                                <option value="">-</option>
                                {vendors.map(v => <option key={v} value={v}>{v}</option>)}
                            </Select>
                        </label>
                        <label class={`flex items-center gap-1.5 ${fieldLabelText(!selectedVendor)}`}>
                            File
                            <Select
                                class="w-56"
                                disabled={!selectedVendor}
                                value={selectedFile}
                                onChange={e => setSelectedFile((e.target as HTMLSelectElement).value)}
                            >
                                <option value="">-</option>
                                {vendorFiles.map(f => <option key={f} value={f}>{f}</option>)}
                            </Select>
                        </label>
                        <Button disabled={!selectedFile} onClick={handleLoad}>Load</Button>
                    </>
                ) : (
                    <Button onClick={handleOpen}>Open...</Button>
                )}
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

            <div class="flex shrink-0 items-center gap-3 px-4 py-2">
                <Button variant="primary" disabled={!sendEnabled} onClick={handleSend}>
                    Send
                </Button>
                {showPort && (
                    <label class={`flex items-center gap-1.5 ${fieldLabelText(!needsPort)}`}>
                        Port
                        <Select
                            class="w-24"
                            disabled={!needsPort}
                            value={selectedPort}
                            onChange={e => setSelectedPort((e.target as HTMLSelectElement).value)}
                        >
                            <option value="">-</option>
                            {ubxPorts.map(p => <option key={p} value={p}>{p}</option>)}
                        </Select>
                    </label>
                )}
                {showSave && (
                    <label class={`flex items-center gap-1.5 ${labeledControlText(!saveAware)}`}>
                        <input
                            type="checkbox"
                            class="accent-accent"
                            disabled={!saveAware}
                            checked={saveAware && save}
                            onChange={e => setSave((e.target as HTMLInputElement).checked)}
                        />
                        Save
                    </label>
                )}
                {sendStatus && (
                    <span class={`text-xs ${sendLines[sendLines.length - 1]?.status === 'error' ? 'text-danger' : 'text-text-secondary'}`}>
                        {sendStatus}
                    </span>
                )}
            </div>

            {/* Two-pane area: left responses, right decode */}
            <div class="flex min-h-[6em] flex-1 gap-2 px-4 pb-4">
                {/* Left pane: response lines */}
                <Card class="flex-1 overflow-y-auto p-2 text-xs leading-relaxed" ref={leftPaneRef}>
                    {hasResults && responseLines.map((r, i) => {
                        const clickable = isClickable(r);
                        const selected = i === selectedResponseIndex;
                        let cls = '';
                        if (r.kind === 'ack' && !r.ackError) cls = 'text-success';
                        else if (r.kind === 'ack' && r.ackError) cls = 'text-danger';
                        else if (r.kind === 'maybe') cls = 'font-mono text-text-muted';
                        else cls = 'font-mono text-text-primary';
                        if (clickable) cls += ' cursor-pointer';
                        if (selected) cls += ' bg-surface-3 rounded';
                        const label = formatResponseLine(r);
                        const showHex = r.kind !== 'ack' && r.bin && (r.tag || r.msgID);
                        return (
                            <div
                                key={i}
                                class={`${cls}${showHex ? ' flex gap-1.5' : ''}`}
                                onClick={clickable ? () => handleResponseClick(i) : undefined}
                            >
                                {showHex ? (
                                    <>
                                        <span class="shrink-0">{label}</span>
                                        <span class="truncate text-text-muted">{r.bin}</span>
                                    </>
                                ) : label}
                            </div>
                        );
                    })}
                    {!hasResults && !tagArmed && (
                        <div class="text-text-muted">Select a tag and click Send</div>
                    )}
                </Card>

                {/* Right pane: decode */}
                <Card class="w-72 shrink-0 flex flex-col overflow-hidden p-2">
                    {selectedResponse ? (
                        <div class="flex-1 overflow-y-auto font-mono text-xs text-text-primary whitespace-pre-wrap">
                            {decodeResult || ''}
                        </div>
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
