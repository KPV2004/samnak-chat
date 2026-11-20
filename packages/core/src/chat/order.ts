import type { ChannelState } from "./channel";

const time = (iso: string | null | undefined) => (iso ? Date.parse(iso) : -Infinity);

/**
 * The order of a conversation list, the one the server returns it in: pinned first (newest pin
 * first), then the newest message, then the title, then the id. `titleOf` supplies the title of a
 * direct message (its channel `name` is empty); it defaults to the channel name.
 */
export function sortConversations(
  items: readonly ChannelState[],
  titleOf: (state: ChannelState) => string = (s) => s.channel.name,
): ChannelState[] {
  return items
    .map((state) => ({ state, title: titleOf(state).toLocaleLowerCase() }))
    .sort((a, b) => {
      const pin = time(b.state.channel.pinned_at) - time(a.state.channel.pinned_at);
      if (pin !== 0 && !Number.isNaN(pin)) return pin;
      const last = time(b.state.channel.last_message?.created_at) - time(a.state.channel.last_message?.created_at);
      if (last !== 0 && !Number.isNaN(last)) return last;
      if (a.title !== b.title) return a.title < b.title ? -1 : 1;
      return a.state.channel.id < b.state.channel.id ? -1 : 1;
    })
    .map((x) => x.state);
}
