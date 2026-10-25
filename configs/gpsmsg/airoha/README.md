# Airoha message files

- [ag33xx.toml](ag33xx.toml) - PAIR commands common to the AG3335 and AG3352
- [ag3335.toml](ag3335.toml) - AG3335 (includes [ag33xx.toml](ag33xx.toml) and [ag3335-add.toml](ag3335-add.toml))
- [ag3352.toml](ag3352.toml) - AG3352 (includes [ag33xx.toml](ag33xx.toml) and [ag3352-add.toml](ag3352-add.toml))
- [ag3335-add.toml](ag3335-add.toml) - PAIR commands only the AG3335 supports
- [ag3352-add.toml](ag3352-add.toml) - PAIR commands only the AG3352 supports

Use ag3335.toml or ag3352.toml for a module that has no message file of its own. The Quectel LC29H and LC76G files in [../quectel](../quectel) include the parts separately.
