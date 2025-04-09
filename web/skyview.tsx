export interface SVInfo {
	azimuth: number;     // 0–360 degrees
	elevation: number;   // 0–90 degrees
	svid: string;        // e.g., "G01"
	cno: number;         // 0 = not tracked
  }
  
  export function SkyView(satellites: SVInfo[]) {
	const SIZE = 200;
	const RADIUS = SIZE / 2;
	const STROKE_PAD = 1;
  
	const toXY = (az: number, el: number): [number, number] => {
	  const r = ((90 - el) / 90) * RADIUS;
	  const rad = (az - 90) * (Math.PI / 180); // 0° = east in SVG
	  const x = RADIUS + r * Math.cos(rad);
	  const y = RADIUS + r * Math.sin(rad);
	  return [x, y];
	};
  
	const tracked = satellites.filter((sv) => sv.cno > 0);
  
	return (
	  <svg
	  viewBox={`${-STROKE_PAD} ${-STROKE_PAD} ${SIZE + 2 * STROKE_PAD} ${SIZE + 2 * STROKE_PAD}`}
		preserveAspectRatio="xMidYMid meet"
		class="w-full h-auto"
		xmlns="http://www.w3.org/2000/svg"
	  >
		<circle cx={RADIUS} cy={RADIUS} r={RADIUS} class="stroke-gray-400 fill-none stroke-[1]" />
		{[15, 30, 45, 60].map((el) => (
		  <circle
			key={el}
			cx={RADIUS}
			cy={RADIUS}
			r={((90 - el) / 90) * RADIUS}
			class="stroke-gray-200 fill-none stroke-[0.5]"
		  />
		))}
		{(() => {
		const outerR = RADIUS; // outermost (0° elevation)
		const innerR = ((90 - 60) / 90) * RADIUS; // innermost (75° elevation)
		return Array.from({ length: 12 }, (_, i) => {
			const angle = i * 30;
			const rad = (angle - 90) * (Math.PI / 180);
			const x1 = RADIUS + outerR * Math.cos(rad);
			const y1 = RADIUS + outerR * Math.sin(rad);
			const x2 = RADIUS + innerR * Math.cos(rad);
			const y2 = RADIUS + innerR * Math.sin(rad);
			return (
			<line
				key={`radial-${angle}`}
				x1={x1}
				y1={y1}
				x2={x2}
				y2={y2}
				class="stroke-gray-300 stroke-[0.5]"
			/>
			);
		});
		})()}
		<text x={RADIUS} y={10} text-anchor="middle" class="fill-gray-500 text-[6px]">N</text>
		{tracked.map((sv) => {
		  const [x, y] = toXY(sv.azimuth, sv.elevation);
		  return (
			<text
			key={sv.svid}
			x={x}
			y={y}
			text-anchor="middle"
			dominant-baseline="middle"
			class={`text-[3px] font-bold ${colorClassFor(sv.svid)} ${opacityClassFor(sv.cno)}`}
			>
			{sv.svid}
			</text>
		  );
		})}
	  </svg>
	);
  }

  function opacityClassFor(cno: number): string {
	if (cno < 25) return 'opacity-20';
	if (cno < 30) return 'opacity-40';
	if (cno < 35) return 'opacity-60';
	if (cno < 42) return 'opacity-75';
	if (cno < 50) return 'opacity-90';
	return 'opacity-100';
  }
  
  function colorClassFor(svid: string): string {
	const prefix = svid[0];
	switch (prefix) {
	  case 'G': // GPS
	  case 'S': // SBAS
		return 'fill-blue-600 dark:fill-blue-400';
	  case 'E': // Galileo
		return 'fill-green-600 dark:fill-green-400';
	  case 'C': // BeiDou
		return 'fill-red-600 dark:fill-red-400';
	  case 'R': // GLONASS
		return 'fill-fuchsia-600 dark:fill-fuchsia-400';
	  case 'J': // QZSS
		return 'fill-orange-600 dark:fill-orange-400';
	  case 'I': // NavIC
		return 'fill-yellow-600 dark:fill-yellow-300';
	  default:
		return 'fill-gray-600 dark:fill-gray-400';
	}
  }