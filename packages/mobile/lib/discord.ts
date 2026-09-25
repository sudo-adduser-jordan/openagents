const OPEN_AGENTS_DISCORD_GUILD_ID = "1476302178913357958";
const OPEN_AGENTS_DISCORD_INVITE_URL = "https://discord.com/invite/UZv7JjxbwG";

/**
 * A Discord channel URL works only for members already signed in to the Open Agents
 * server. Until the feature-request channel ID is configured, the official
 * invite is the reliable cross-platform destination.
 */
export function discordFeatureRequestURL(featureRequestChannelId?: string): string {
	return featureRequestChannelId
		? `https://discord.com/channels/${OPEN_AGENTS_DISCORD_GUILD_ID}/${featureRequestChannelId}`
		: OPEN_AGENTS_DISCORD_INVITE_URL;
}
