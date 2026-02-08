import {h, Fragment} from 'preact';

export type ReceiverState =
    | {status: 'disconnected'}
    | {status: 'probing'}
    | {status: 'identified'; vendor: string; hardware: string; firmware: string; supportedGNSS: string[]; packetFormats: string[]}
    | {status: 'error'; error: string};

interface Props {
    receiver: ReceiverState;
}

export function ReceiverPanel({receiver}: Props) {
    return (
        <div class="p-4 overflow-y-auto h-full">
            <h2 class="text-base font-semibold mb-4">Receiver</h2>

            {receiver.status === 'disconnected' && (
                <p class="text-gray-500 dark:text-gray-400 italic text-sm">Not connected</p>
            )}

            {receiver.status === 'probing' && (
                <p class="text-gray-500 dark:text-gray-400 text-sm">Identifying receiver...</p>
            )}

            {receiver.status === 'error' && (
                <p class="text-red-600 dark:text-red-400 text-sm">{receiver.error}</p>
            )}

            {receiver.status === 'identified' && (
                <dl class="grid grid-cols-[120px_1fr] gap-x-4 gap-y-2 max-w-xl">
                    {receiver.vendor && (<>
                        <dt class="text-gray-500 dark:text-gray-400 text-xs">Vendor</dt>
                        <dd class="text-sm">{receiver.vendor}</dd>
                    </>)}
                    {receiver.hardware && (<>
                        <dt class="text-gray-500 dark:text-gray-400 text-xs">Hardware</dt>
                        <dd class="text-sm">{receiver.hardware}</dd>
                    </>)}
                    {receiver.firmware && (<>
                        <dt class="text-gray-500 dark:text-gray-400 text-xs">Firmware</dt>
                        <dd class="text-sm">{receiver.firmware}</dd>
                    </>)}
                    {receiver.supportedGNSS?.length > 0 && (<>
                        <dt class="text-gray-500 dark:text-gray-400 text-xs">GNSS</dt>
                        <dd class="text-sm">{receiver.supportedGNSS.join(', ')}</dd>
                    </>)}
                    {receiver.packetFormats?.length > 0 && (<>
                        <dt class="text-gray-500 dark:text-gray-400 text-xs">Protocols</dt>
                        <dd class="text-sm">{receiver.packetFormats.join(', ')}</dd>
                    </>)}
                </dl>
            )}
        </div>
    );
}
