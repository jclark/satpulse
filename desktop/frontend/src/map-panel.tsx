import {h} from 'preact';
import {useMemo} from 'preact/hooks';
import {BrowserOpenURL} from '../wailsjs/runtime/runtime';

interface MapPanelProps {
    pos: {lat: number; lon: number} | null;
    noFixSecs: number;
}

// Standard OSM slippy map tile math.
function latLonToTile(lat: number, lon: number, zoom: number) {
    const n = 1 << zoom;
    const latRad = (lat * Math.PI) / 180;
    const xFloat = ((lon + 180) / 360) * n;
    const yFloat = ((1 - Math.log(Math.tan(latRad) + 1 / Math.cos(latRad)) / Math.PI) / 2) * n;
    const tileX = Math.floor(xFloat);
    const tileY = Math.floor(yFloat);
    const pixelX = Math.floor((xFloat - tileX) * 256);
    const pixelY = Math.floor((yFloat - tileY) * 256);
    return {tileX, tileY, pixelX, pixelY};
}

const ZOOM = 16;
const SIZE = 256;
const HALF = SIZE / 2; // 128

export function MapPanel({pos, noFixSecs}: MapPanelProps) {
    const tileInfo = useMemo(() => {
        if (!pos) return null;
        const {tileX, tileY, pixelX, pixelY} = latLonToTile(pos.lat, pos.lon, ZOOM);
        // 2x2 tile grid (512x512) cropped to 256x256 viewport centered on the dot.
        // The dot's tile is always top-left; adjacent tiles fill the rest.
        // Position the grid so the dot pixel (pixelX, pixelY) maps to viewport center.
        const gridLeft = HALF - pixelX;
        const gridTop = HALF - pixelY;
        return {tileX, tileY, gridLeft, gridTop};
    }, [pos?.lat, pos?.lon]);

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
                class="relative bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-gray-400 dark:text-gray-500 text-sm select-none shrink-0"
                style={{width: SIZE + 'px', height: SIZE + 'px'}}
            >
                Waiting for position
            </div>
        );
    }

    return (
        <div
            class="relative overflow-hidden cursor-pointer shrink-0"
            style={{width: SIZE + 'px', height: SIZE + 'px'}}
            onClick={openGoogleMaps}
            title="Click to open in Google Maps"
        >
            {/* Tile grid */}
            {tileInfo && (
                <div
                    style={{
                        position: 'absolute',
                        left: tileInfo.gridLeft + 'px',
                        top: tileInfo.gridTop + 'px',
                        width: '512px',
                        height: '512px',
                    }}
                >
                    {[0, 1].map(dy =>
                        [0, 1].map(dx => (
                            <img
                                key={`${tileInfo.tileX + dx},${tileInfo.tileY + dy}`}
                                src={`https://tile.openstreetmap.org/${ZOOM}/${tileInfo.tileX + dx}/${tileInfo.tileY + dy}.png`}
                                style={{
                                    position: 'absolute',
                                    left: dx * 256 + 'px',
                                    top: dy * 256 + 'px',
                                    width: '256px',
                                    height: '256px',
                                    display: 'block',
                                }}
                                draggable={false}
                            />
                        ))
                    )}
                </div>
            )}

            {/* Crosshair marker at center */}
            <div
                class="absolute pointer-events-none"
                style={{left: HALF + 'px', top: HALF + 'px', transform: 'translate(-50%, -50%)'}}
            >
                <svg width="24" height="24" viewBox="0 0 24 24">
                    <circle cx="12" cy="12" r="5" fill="rgba(59,130,246,0.8)" stroke="white" stroke-width="2" />
                    <line x1="12" y1="0" x2="12" y2="8" stroke="white" stroke-width="1.5" opacity="0.8" />
                    <line x1="12" y1="16" x2="12" y2="24" stroke="white" stroke-width="1.5" opacity="0.8" />
                    <line x1="0" y1="12" x2="8" y2="12" stroke="white" stroke-width="1.5" opacity="0.8" />
                    <line x1="16" y1="12" x2="24" y2="12" stroke="white" stroke-width="1.5" opacity="0.8" />
                </svg>
            </div>

            {/* No fix overlay */}
            {noFixSecs > 0 && (
                <div class="absolute inset-0 flex items-center justify-center bg-black/40">
                    <span class="text-white text-sm font-medium px-3 py-1 rounded bg-black/60">
                        No fix for {noFixSecs} s
                    </span>
                </div>
            )}

            {/* OSM attribution */}
            <div class="absolute bottom-0 right-0 text-[10px] text-gray-600 bg-white/70 px-1">
                &copy; OpenStreetMap contributors
            </div>
        </div>
    );
}
