import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {BrowserOpenURL} from '../wailsjs/runtime/runtime';

interface MapPanelProps {
    pos: {lat: number; lon: number} | null;
    course: {course: number; groundSpeed: number} | null;
    noFixSecs: number;
}

// Convert lat/lon to global pixel coordinates at the given zoom level.
function latLonToPixel(lat: number, lon: number, zoom: number): {x: number; y: number} {
    const n = 1 << zoom;
    const latRad = (lat * Math.PI) / 180;
    return {
        x: ((lon + 180) / 360) * n * 256,
        y: ((1 - Math.log(Math.tan(latRad) + 1 / Math.cos(latRad)) / Math.PI) / 2) * n * 256,
    };
}

const ZOOM = 17;
const TILE = 256;
const MARGIN = 40; // re-center when dot is this close to viewport edge

export function MapPanel({pos, course, noFixSecs}: MapPanelProps) {
    const containerRef = useRef<HTMLDivElement>(null);
    const anchorRef = useRef<{x: number; y: number} | null>(null);
    const [dims, setDims] = useState({w: 0, h: 0});

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;
        const update = () => {
            const r = el.getBoundingClientRect();
            const w = Math.round(r.width);
            const h = Math.round(r.height);
            if (w > 0 && h > 0) setDims(prev => (prev.w === w && prev.h === h ? prev : {w, h}));
        };
        update();
        const ro = new ResizeObserver(update);
        ro.observe(el);
        return () => ro.disconnect();
    }, []);

    const mapState = useMemo(() => {
        const {w, h} = dims;
        if (!pos || w === 0 || h === 0) return null;
        const halfW = w / 2;
        const halfH = h / 2;
        const dot = latLonToPixel(pos.lat, pos.lon, ZOOM);
        let anchor = anchorRef.current;
        if (!anchor) {
            anchor = {x: dot.x, y: dot.y};
            anchorRef.current = anchor;
        } else {
            // Re-center when dot approaches the viewport edge.
            const dvx = halfW + dot.x - anchor.x;
            const dvy = halfH + dot.y - anchor.y;
            if (dvx < MARGIN || dvx > w - MARGIN || dvy < MARGIN || dvy > h - MARGIN) {
                anchor = {x: dot.x, y: dot.y};
                anchorRef.current = anchor;
            }
        }
        // Dot position in viewport pixels.
        const dotLeft = Math.round(halfW + dot.x - anchor.x);
        const dotTop = Math.round(halfH + dot.y - anchor.y);
        // Tile grid: cover the viewport with however many 256px tiles are needed.
        const vpLeft = anchor.x - halfW;
        const vpTop = anchor.y - halfH;
        const baseTileX = Math.floor(vpLeft / TILE);
        const baseTileY = Math.floor(vpTop / TILE);
        const lastTileX = Math.floor((vpLeft + w - 1) / TILE);
        const lastTileY = Math.floor((vpTop + h - 1) / TILE);
        const cols = lastTileX - baseTileX + 1;
        const rows = lastTileY - baseTileY + 1;
        const gridLeft = Math.round(baseTileX * TILE - vpLeft);
        const gridTop = Math.round(baseTileY * TILE - vpTop);
        return {baseTileX, baseTileY, cols, rows, gridLeft, gridTop, dotLeft, dotTop};
    }, [pos?.lat, pos?.lon, dims.w, dims.h]);

    const openGoogleMaps = () => {
        if (pos) {
            const q = encodeURIComponent(`${pos.lat.toFixed(7)},${pos.lon.toFixed(7)}`);
            BrowserOpenURL(`https://www.google.com/maps/search/?api=1&query=${q}`);
        }
    };

    // No position yet
    if (!pos) {
        return (
            <div
                ref={containerRef}
                class="relative flex h-full w-full select-none items-center justify-center bg-surface-1 text-sm text-text-muted"
            >
                Waiting for position
            </div>
        );
    }

    return (
        <div
            ref={containerRef}
            class="relative h-full w-full overflow-hidden cursor-pointer"
            onClick={openGoogleMaps}
            title="Click to open in Google Maps"
        >
            {/* Tile grid */}
            {mapState && (
                <div
                    style={{
                        position: 'absolute',
                        left: mapState.gridLeft + 'px',
                        top: mapState.gridTop + 'px',
                        width: mapState.cols * TILE + 'px',
                        height: mapState.rows * TILE + 'px',
                    }}
                >
                    {Array.from({length: mapState.rows}, (_, dy) =>
                        Array.from({length: mapState.cols}, (_, dx) => (
                            <img
                                key={`${mapState.baseTileX + dx},${mapState.baseTileY + dy}`}
                                src={`https://tile.openstreetmap.org/${ZOOM}/${mapState.baseTileX + dx}/${mapState.baseTileY + dy}.png`}
                                style={{
                                    position: 'absolute',
                                    left: dx * TILE + 'px',
                                    top: dy * TILE + 'px',
                                    width: TILE + 'px',
                                    height: TILE + 'px',
                                    display: 'block',
                                }}
                                draggable={false}
                            />
                        ))
                    )}
                </div>
            )}

            {/* Position marker */}
            {mapState && (
            <div
                class="absolute pointer-events-none"
                style={{left: mapState.dotLeft + 'px', top: mapState.dotTop + 'px', transform: 'translate(-50%, -50%)'}}
            >
                {course && course.groundSpeed >= 0.1 ? (
                    <svg width="24" height="24" viewBox="0 0 24 24" style={{transform: `rotate(${course.course}deg)`}}>
                        <polygon points="12,2 4,20 12,16 20,20" fill="var(--accent)" fill-opacity="0.8" stroke="var(--surface-2)" stroke-width="1.5" stroke-linejoin="round" />
                    </svg>
                ) : (
                    <svg width="24" height="24" viewBox="0 0 24 24">
                        <circle cx="12" cy="12" r="5" fill="var(--accent)" fill-opacity="0.8" stroke="var(--surface-2)" stroke-width="2" />
                        <line x1="12" y1="0" x2="12" y2="8" stroke="var(--surface-2)" stroke-width="1.5" opacity="0.8" />
                        <line x1="12" y1="16" x2="12" y2="24" stroke="var(--surface-2)" stroke-width="1.5" opacity="0.8" />
                        <line x1="0" y1="12" x2="8" y2="12" stroke="var(--surface-2)" stroke-width="1.5" opacity="0.8" />
                        <line x1="16" y1="12" x2="24" y2="12" stroke="var(--surface-2)" stroke-width="1.5" opacity="0.8" />
                    </svg>
                )}
            </div>
            )}

            {/* No fix overlay */}
            {noFixSecs > 0 && (
                <div class="absolute inset-0 flex items-center justify-center bg-surface-1/40">
                    <span class="rounded bg-surface-1/70 px-3 py-1 text-sm font-medium text-text-primary">
                        No fix for {noFixSecs} s
                    </span>
                </div>
            )}

            {/* OSM attribution */}
            <div class="absolute bottom-0 right-0 bg-surface-2/70 px-1 text-[10px] text-text-secondary">
                &copy; OpenStreetMap contributors
            </div>
        </div>
    );
}
