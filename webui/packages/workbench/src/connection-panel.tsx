import {h} from 'preact';
import {useState, useRef, useEffect, useCallback} from 'preact/hooks';
import {Button, Input, Select} from './ui';
import {speeds} from './speeds';

import type {PortInfo} from './transport';
export type {PortInfo};

interface Props {
    connected: boolean;
    readOnly: boolean;
    onUseHere: () => void;
    device: string;
    setDevice: (d: string) => void;
    speed: number;
    setSpeed: (s: number) => void;
    onConnect: () => void;
    receiverIdent: string;
    ports: PortInfo[];
    onRefreshPorts: () => void;
    onChoosePort?: () => void;
}

export function ConnectionPanel({
    connected,
    readOnly,
    onUseHere,
    device,
    setDevice,
    speed,
    setSpeed,
    onConnect,
    receiverIdent,
    ports,
    onRefreshPorts,
    onChoosePort,
}: Props) {
    const [open, setOpen] = useState(false);
    const wrapRef = useRef<HTMLDivElement>(null);
    // Choosing from the list unmounts the focused entry; keep focus in the bar.
    const focusToggle = () => wrapRef.current?.querySelector<HTMLButtonElement>('button[aria-label="Select port"]')?.focus();

    useEffect(() => {
        if (!open) return;
        const handler = (e: MouseEvent) => {
            if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
                setOpen(false);
            }
        };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, [open]);

    const toggleDropdown = useCallback(() => {
        if (!open) {
            onRefreshPorts();
        }
        setOpen(v => !v);
    }, [open, onRefreshPorts]);

    const selectPort = useCallback(
        (p: PortInfo) => {
            setDevice(p.device);
            setOpen(false);
            focusToggle();
        },
        [setDevice],
    );

    return (
        <header class="flex shrink-0 items-center gap-4 border-b border-border-subtle bg-surface-2 px-5 py-3">
            <h1 class="text-base font-semibold whitespace-nowrap text-text-primary">SatPulse</h1>
            {receiverIdent && <span class="text-xs whitespace-nowrap text-text-secondary">{receiverIdent}</span>}
            {readOnly && (
                <div class="flex items-center gap-2">
                    <span class="text-xs text-warning">Read-only: this workbench has been taken over by another window</span>
                    <Button size="sm" onClick={onUseHere}>Use here</Button>
                </div>
            )}
            <div class="ml-auto flex items-center gap-2">
                <div class={`h-2.5 w-2.5 shrink-0 rounded-full ${connected ? 'bg-success' : 'bg-text-muted'}`} />
                <label class="text-xs text-text-secondary">Device</label>
                <div class="relative" ref={wrapRef}>
                    <div class="flex">
                        <Input
                            type="text"
                            class="w-44 rounded-r-none border-r-0"
                            value={device}
                            disabled={connected || readOnly}
                            onInput={e => setDevice((e.target as HTMLInputElement).value)}
                            placeholder={onChoosePort ? 'select a device' : 'device path'}
                            readOnly={!!onChoosePort}
                        />
                        <Button
                            type="button"
                            variant="secondary"
                            class="rounded-l-none px-1.5"
                            onClick={toggleDropdown}
                            disabled={connected || readOnly}
                            aria-label="Select port"
                        >
                            <svg class="h-3 w-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M3 5l3 3 3-3" />
                            </svg>
                        </Button>
                    </div>
                    {open && (
                        <ul class="absolute right-0 left-0 z-50 mt-0.5 max-h-48 overflow-y-auto rounded border border-border-subtle bg-surface-2 shadow-sm">
                            {ports.length === 0 && <li class="px-2 py-1.5 text-xs italic text-text-muted">{onChoosePort ? 'No devices added' : 'No ports found'}</li>}
                            {ports.map(p => (
                                <li key={p.device}>
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        class="w-full justify-start rounded-none"
                                        disabled={connected || readOnly}
                                        onClick={() => selectPort(p)}
                                    >
                                        {p.display}
                                    </Button>
                                </li>
                            ))}
                            {onChoosePort && (
                                <li>
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        class="w-full justify-start rounded-none"
                                        disabled={connected || readOnly}
                                        onClick={() => {
                                            setOpen(false);
                                            onChoosePort();
                                            focusToggle();
                                        }}
                                    >
                                        Add a device...
                                    </Button>
                                </li>
                            )}
                        </ul>
                    )}
                </div>
                <label class="text-xs text-text-secondary">Speed</label>
                <Select
                    class="w-24"
                    value={speed}
                    disabled={connected || readOnly}
                    onChange={e => setSpeed(parseInt((e.target as HTMLSelectElement).value, 10))}
                >
                    {speeds.map(s => (
                        <option key={s} value={s}>
                            {s}
                        </option>
                    ))}
                </Select>
                <Button
                    variant={connected ? 'secondary' : 'primary'}
                    class="whitespace-nowrap"
                    onClick={onConnect}
                    disabled={readOnly || (!connected && !device)}
                >
                    {connected ? 'Disconnect' : 'Connect'}
                </Button>
            </div>
        </header>
    );
}

// DeviceGoneDialog offers to choose the device again after it disappeared,
// for example when a receiver reset re-enumerated its USB interface. It is a
// modal dialog, so the page behind it is inert and Escape cancels it.
export function DeviceGoneDialog({onReconnect, onCancel}: {onReconnect: () => void; onCancel: () => void}) {
    const ref = useRef<HTMLDialogElement>(null);
    useEffect(() => {
        const d = ref.current!;
        d.showModal();
        d.querySelector<HTMLButtonElement>('button:last-child')?.focus();
        return () => d.close();
    }, []);
    return (
        <dialog
            ref={ref}
            aria-labelledby="device-gone-title"
            class="m-auto w-110 max-w-[90vw] rounded-lg border border-border-subtle bg-surface-2 p-0 text-sm backdrop:bg-surface-1/70"
            onCancel={e => { e.preventDefault(); onCancel(); }}
            onClick={e => { if (e.target === ref.current) onCancel(); }}
        >
            <p id="device-gone-title" class="m-0 px-4 py-3 text-text-primary">The receiver's serial device has disconnected.</p>
            <div class="flex justify-end gap-2 border-t border-border-subtle px-4 py-3">
                <Button onClick={onCancel}>Cancel</Button>
                <Button variant="primary" onClick={onReconnect}>Reconnect...</Button>
            </div>
        </dialog>
    );
}
