import {h} from 'preact';
import {useState, useRef, useEffect, useCallback} from 'preact/hooks';
import {Button, Input, Select} from './ui';

export interface PortInfo {
    device: string;
    display: string;
}

interface Props {
    connected: boolean;
    device: string;
    setDevice: (d: string) => void;
    speed: number;
    setSpeed: (s: number) => void;
    onConnect: () => void;
    receiverIdent: string;
    ports: PortInfo[];
    onRefreshPorts: () => void;
}

const speeds = [9600, 38400, 57600, 115200, 230400, 460800, 921600];

export function ConnectionPanel({
    connected,
    device,
    setDevice,
    speed,
    setSpeed,
    onConnect,
    receiverIdent,
    ports,
    onRefreshPorts,
}: Props) {
    const [open, setOpen] = useState(false);
    const wrapRef = useRef<HTMLDivElement>(null);

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
        },
        [setDevice],
    );

    return (
        <header class="flex shrink-0 items-center gap-4 border-b border-border-subtle bg-surface-2 px-5 py-3">
            <h1 class="text-base font-semibold whitespace-nowrap text-text-primary">SatPulse</h1>
            {receiverIdent && <span class="text-xs whitespace-nowrap text-text-secondary">{receiverIdent}</span>}
            <div class="ml-auto flex items-center gap-2">
                <div class={`h-2.5 w-2.5 shrink-0 rounded-full ${connected ? 'bg-success' : 'bg-text-muted'}`} />
                <label class="text-xs text-text-secondary">Device</label>
                <div class="relative" ref={wrapRef}>
                    <div class="flex">
                        <Input
                            type="text"
                            class="w-44 rounded-r-none border-r-0"
                            value={device}
                            onInput={e => setDevice((e.target as HTMLInputElement).value)}
                            placeholder="device path"
                        />
                        <Button
                            type="button"
                            variant="secondary"
                            class="rounded-l-none px-1.5"
                            onClick={toggleDropdown}
                            aria-label="Select port"
                        >
                            <svg class="h-3 w-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M3 5l3 3 3-3" />
                            </svg>
                        </Button>
                    </div>
                    {open && (
                        <ul class="absolute right-0 left-0 z-50 mt-0.5 max-h-48 overflow-y-auto rounded border border-border-subtle bg-surface-2 shadow-sm">
                            {ports.length === 0 && <li class="px-2 py-1.5 text-xs italic text-text-muted">No ports found</li>}
                            {ports.map(p => (
                                <li
                                    key={p.device}
                                    class="cursor-pointer px-2 py-1.5 text-xs text-text-primary hover:bg-surface-3"
                                    onClick={() => selectPort(p)}
                                >
                                    {p.display}
                                </li>
                            ))}
                        </ul>
                    )}
                </div>
                <label class="text-xs text-text-secondary">Speed</label>
                <Select
                    class="w-24"
                    value={speed}
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
                    disabled={!connected && !device}
                >
                    {connected ? 'Disconnect' : 'Connect'}
                </Button>
            </div>
        </header>
    );
}
