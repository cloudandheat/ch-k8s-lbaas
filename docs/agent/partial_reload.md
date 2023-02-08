# Partial Reload

The partial-reload feature for nftables can be used if there is no `nftables` service or similar that can just be reloaded.

When the option is enabled, the behaviour of the agent changes as follows:

- The nftables config is reloaded on start of the agent, so that the last config is applied
- When generating the nftables config
    - `flush chain` statements are rendered for `FilterForwardChainName`, `NATPreroutingChainName` and `NATPostroutingChainName`
    - `delete chain` statements are rendered for all currently existing chains in the `FilterTableName` table starting with `PolicyPrefix`

These changes allow lbaas to run in an environment where it isn't possible to reload the complete nftables ruleset.
In this case, the changes can be applied with `nft -f`.

## Example config
An example configuration for partial-reload usage on VyOS can be found [here](environments/vyos.md).
