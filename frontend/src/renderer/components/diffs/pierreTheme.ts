// Pierre derives its canvas from the selected Shiki theme and injects that
// value inside its shadow root. Keep the theme's syntax colors, but use Open Agents's
// own workspace canvas for the visible file and diff surfaces.
export const OPEN_AGENTS_PIERRE_SURFACE_CSS = `
:host {
	--diffs-bg: var(--color-bg-primary);
}
`;
