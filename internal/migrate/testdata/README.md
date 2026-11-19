# Migration fixtures

Every file in this directory is synthetic. The domains use the reserved
`.example` / `.invalid` names, the addresses are from the documentation ranges,
and every credential-shaped value is a placeholder. Nothing here is copied from
a real Pi-hole or AdGuard Home installation.

The fixtures exist so the importer tests can pin three things without a network
or a live source host:

- the AdGuard Home schema variants (`clients.persistent` and the older
  `clients` key), including secret fields that must be redacted;
- the Pi-hole v5 Teleporter text files (`gravity`, allow/block lists, custom
  DNS, setup variables and adlists);
- the reduced `gravity.db` table set a Pi-hole v6 Teleporter carries.

`pihole-v6-gravity.sql` is the source for the optional `pihole-v6-gravity.db`
fixture. Keep the two in sync when adding a table the parser reads.
