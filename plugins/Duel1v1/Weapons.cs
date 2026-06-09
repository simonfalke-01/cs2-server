namespace Duel1v1;

/// <summary>
/// Weapon catalog: maps chat-command aliases (e.g. "ak") to CS2 weapon designer
/// names (e.g. "weapon_ak47"). Single source of truth — also drives the !guns
/// listing and the set of registered weapon commands.
/// </summary>
public static class Weapons
{
    public enum Slot { Primary, Secondary }

    public record Entry(string Alias, string DesignerName, Slot Slot, string Display);

    // Full CS2 arsenal. Aliases are the chat commands (!ak, !awp, ...).
    public static readonly Entry[] All =
    {
        // --- Rifles (primary) ---
        new("ak",       "weapon_ak47",        Slot.Primary, "AK-47"),
        new("m4",       "weapon_m4a1",        Slot.Primary, "M4A4"),
        new("m4a4",     "weapon_m4a1",        Slot.Primary, "M4A4"),
        new("m4s",      "weapon_m4a1_silencer", Slot.Primary, "M4A1-S"),
        new("m4a1s",    "weapon_m4a1_silencer", Slot.Primary, "M4A1-S"),
        new("aug",      "weapon_aug",         Slot.Primary, "AUG"),
        new("sg",       "weapon_sg556",       Slot.Primary, "SG 553"),
        new("sg553",    "weapon_sg556",       Slot.Primary, "SG 553"),
        new("galil",    "weapon_galilar",     Slot.Primary, "Galil AR"),
        new("famas",    "weapon_famas",       Slot.Primary, "FAMAS"),

        // --- Snipers (primary) ---
        new("awp",      "weapon_awp",         Slot.Primary, "AWP"),
        new("scout",    "weapon_ssg08",       Slot.Primary, "SSG 08 (Scout)"),
        new("ssg",      "weapon_ssg08",       Slot.Primary, "SSG 08 (Scout)"),
        new("auto",     "weapon_g3sg1",       Slot.Primary, "G3SG1 / SCAR-20"),
        new("scar",     "weapon_scar20",      Slot.Primary, "SCAR-20"),
        new("g3",       "weapon_g3sg1",       Slot.Primary, "G3SG1"),

        // --- SMGs (primary) ---
        new("mp9",      "weapon_mp9",         Slot.Primary, "MP9"),
        new("mac10",    "weapon_mac10",       Slot.Primary, "MAC-10"),
        new("mp7",      "weapon_mp7",         Slot.Primary, "MP7"),
        new("mp5",      "weapon_mp5sd",       Slot.Primary, "MP5-SD"),
        new("ump",      "weapon_ump45",       Slot.Primary, "UMP-45"),
        new("p90",      "weapon_p90",         Slot.Primary, "P90"),
        new("bizon",    "weapon_bizon",       Slot.Primary, "PP-Bizon"),

        // --- Heavy (primary) ---
        new("nova",     "weapon_nova",        Slot.Primary, "Nova"),
        new("xm",       "weapon_xm1014",      Slot.Primary, "XM1014"),
        new("xm1014",   "weapon_xm1014",      Slot.Primary, "XM1014"),
        new("mag7",     "weapon_mag7",        Slot.Primary, "MAG-7"),
        new("sawedoff", "weapon_sawedoff",    Slot.Primary, "Sawed-Off"),
        new("m249",     "weapon_m249",        Slot.Primary, "M249"),
        new("negev",    "weapon_negev",       Slot.Primary, "Negev"),

        // --- Pistols (secondary) ---
        new("deagle",   "weapon_deagle",      Slot.Secondary, "Desert Eagle"),
        new("r8",       "weapon_revolver",    Slot.Secondary, "R8 Revolver"),
        new("revolver", "weapon_revolver",    Slot.Secondary, "R8 Revolver"),
        new("glock",    "weapon_glock",       Slot.Secondary, "Glock-18"),
        new("usp",      "weapon_usp_silencer", Slot.Secondary, "USP-S"),
        new("p2000",    "weapon_hkp2000",     Slot.Secondary, "P2000"),
        new("p250",     "weapon_p250",        Slot.Secondary, "P250"),
        new("tec9",     "weapon_tec9",        Slot.Secondary, "Tec-9"),
        new("fiveseven","weapon_fiveseven",   Slot.Secondary, "Five-SeveN"),
        new("cz",       "weapon_cz75a",       Slot.Secondary, "CZ75-Auto"),
        new("dualies",  "weapon_elite",       Slot.Secondary, "Dual Berettas"),
        new("elite",    "weapon_elite",       Slot.Secondary, "Dual Berettas"),
    };

    // Default loadout (used until a player picks otherwise).
    public const string DefaultPrimary = "weapon_ak47";
    public const string DefaultSecondary = "weapon_deagle";

    // Standard grenade kit given each round when nades are enabled.
    public static readonly string[] NadeKit =
    {
        "weapon_hegrenade",
        "weapon_flashbang",
        "weapon_smokegrenade",
        "weapon_molotov",
    };

    /// <summary>Resolve a chat alias (case-insensitive) to a catalog entry.</summary>
    public static Entry? Resolve(string alias)
    {
        alias = alias.Trim().ToLowerInvariant();
        foreach (var e in All)
        {
            if (e.Alias == alias) return e;
        }
        return null;
    }

    /// <summary>Distinct aliases for the !guns listing, grouped by slot.</summary>
    public static (string Primaries, string Secondaries) Listing()
    {
        var primaries = new List<string>();
        var secondaries = new List<string>();
        var seen = new HashSet<string>();
        foreach (var e in All)
        {
            if (!seen.Add(e.Alias)) continue;
            if (e.Slot == Slot.Primary) primaries.Add("!" + e.Alias);
            else secondaries.Add("!" + e.Alias);
        }
        return (string.Join(" ", primaries), string.Join(" ", secondaries));
    }
}
