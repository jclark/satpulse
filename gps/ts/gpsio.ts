import type {Tag} from './gpsprot';

export interface PacketLogEntry {
    t: string;
    tag?: Tag;
    msg?: string;
    more?: boolean;
    bin?: string;
    ascii?: string;
    speed?: number;
    out?: boolean;
}
