import {
	AgentAvatar as ProductAgentAvatar,
	type AgentAvatarProps as ProductAgentAvatarProps,
	type AgentLogoSources,
} from "@openagents/product-ui";
import museLogo from "../assets/agents/muse.svg";
import nemotronLogo from "../assets/agents/nemotron.svg";
import opencodeLogo from "../assets/agents/opencode.svg";
import { resolveModelLogoKey } from "../lib/model-logo";

// Real brand logo keyed by the harness name Open Agents stores on session.provider.
// Agents without an asset fall back to a lettered tile. The `model-*` keys are
// not harness names; they are the keys `resolveModelLogoKey` hands back, passed
// through as `logoSrc` so a model's own mark outranks its harness's.
const LOGOS: AgentLogoSources = {
	opencode: opencodeLogo,
	"model-muse": museLogo,
	"model-nemotron": nemotronLogo,
};

/**
 * Agent mark for board/task cards: the harness's real brand logo rendered bare —
 * no tile, border, or background — so each brand's own shape shows agents
 * carrying their own rounded background. Agents without an asset fall back to a
 * bare initial. Kept small so the title stays the hero.
 *
 * The provider is exposed as the accessible name (alt / aria-label), not just a
 * hover title, so surfaces that show the logo in place of visible agent text —
 * e.g. the archive cards — still name the agent for screen readers.
 *
 * Pass `model` to show the model's own brand mark instead of the harness's,
 * which is what every live session surface wants: the mark should track the
 * model actually answering, including a reroute. The harness keeps supplying
 * the accessible name, so a model mark never reads as a differently-named
 * agent.
 */
export function AgentAvatar({ provider, model, className, decorative = false }: AgentAvatarProps) {
	const logoKey = resolveModelLogoKey(model);
	return (
		<ProductAgentAvatar
			className={className}
			decorative={decorative}
			logoSrc={logoKey ? LOGOS[logoKey] : undefined}
			logoSources={LOGOS}
			provider={provider}
		/>
	);
}

export type AgentAvatarProps = ProductAgentAvatarProps & {
	/** Model id for this session; resolves to a brand mark, else the harness's. */
	model?: string;
};
