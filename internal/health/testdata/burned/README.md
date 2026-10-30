# Burned test key — DO NOT USE

A copy of `isotope/testdata/burned/`, the lab's deliberately burned keypair owned
by **Zaphod Beeblebrox**. The private half is published on purpose: anything
signed with it is worthless, which is what makes it a reproducible vector rather
than an identity.

It is used here for the certificate path (isotope#25, isotope#26): the alarm pins
this key as a root, a cluster presents a certificate it issued, and the receipt
verifies — with no generated key standing in for the vector, and no network.

| file | what it is |
| --- | --- |
| `id_ed25519` | the burned private key (OpenSSH format, no passphrase) |
| `id_ed25519.pub` | its public half |

The tests assert that the private half is the published half's pair and that its
fingerprint is `SHA256:YzfLqN67xkxrW/BxBnrVTSNDIzrpDEYt0/Twc8jUjOo`, so a
regenerated key shows up as a diff instead of quietly changing what the vector
means.

Source: `isotope/testdata/burned/` (readme there explains the full model).
