using System.Text.RegularExpressions;

namespace Duel1v1;

/// <summary>
/// Curated 1v1 map pool used by !maps / !map / !votemap. Entries are either
/// stock maps (changed via "changelevel") or Steam Workshop maps (changed via
/// "host_workshop_map" using the workshop file id — the command that loads a
/// workshop map BY its numeric id, downloading it if missing; ds_workshop_changelevel
/// takes a map NAME from a loaded collection, which is why passing an id failed).
///
/// Players are never restricted to this pool — /create map: and an explicit
/// !map &lt;name&gt; can load any map. This is just the convenient shortlist.
/// </summary>
public static class MapPool
{
    public record MapEntry(string Name, string Display, ulong WorkshopId)
    {
        public bool IsWorkshop => WorkshopId != 0;
    }

    // Stock 1v1/aim-friendly maps ship with CS2; workshop entries carry an id.
    // TODO: the curated workshop ids below need verification against the live
    // Workshop — id 3071005299 was mislabeled "aim_redline" but actually loads
    // de_assembly (corrected here); the other ids are assumed and unverified.
    public static readonly MapEntry[] All =
    {
        new("aim_map",        "aim_map (workshop)",      3070253702),
        new("aim_botz",       "aim_botz (workshop)",     3084291314),
        new("1v1_arena",      "1v1 Arena (workshop)",    3340432449),
        new("de_assembly",    "de_assembly (workshop)",  3071005299),
        // Stock maps (no workshop id -> changelevel).
        new("de_dust2",       "Dust II",                 0),
        new("de_mirage",      "Mirage",                  0),
        new("de_inferno",     "Inferno",                 0),
    };

    // A map name/id is only ever forwarded to a server console command
    // (changelevel/host_workshop_map). Player-supplied text must therefore be
    // strictly validated so it cannot chain extra console commands (e.g. via ';'
    // or whitespace) through ExecuteCommand. Pool entries always pass because
    // they are matched by Resolve before this is consulted.
    private static readonly Regex MapNameRe = new("^[a-zA-Z0-9_]+$", RegexOptions.Compiled);

    /// <summary>
    /// True if <paramref name="name"/> is safe to forward to a changelevel /
    /// host_workshop_map console command: either a known pool entry, an all-digit
    /// workshop id, or a plain map name matching ^[a-zA-Z0-9_]+$ (no ';',
    /// whitespace, or quotes that could chain console commands).
    /// </summary>
    public static bool IsSafeName(string name)
    {
        if (string.IsNullOrWhiteSpace(name)) return false;
        name = name.Trim();
        // Known pool entries are always safe.
        if (Resolve(name) != null) return true;
        // Workshop ids are all-digits; arbitrary map names are [a-zA-Z0-9_].
        return MapNameRe.IsMatch(name);
    }

    public static MapEntry? Resolve(string name)
    {
        name = name.Trim().ToLowerInvariant();
        foreach (var m in All)
        {
            if (m.Name.ToLowerInvariant() == name) return m;
        }
        return null;
    }

    public static string Listing()
    {
        var names = new List<string>();
        foreach (var m in All) names.Add(m.Name);
        return string.Join(", ", names);
    }

    /// <summary>The server console command that switches to this map.</summary>
    public static string ChangeCommand(MapEntry m)
        => m.IsWorkshop ? $"host_workshop_map {m.WorkshopId}" : $"changelevel {m.Name}";

    /// <summary>
    /// Change command for an arbitrary, possibly off-pool, map name. Callers MUST
    /// gate the name through <see cref="IsSafeName"/> first — this assumes the
    /// input is already validated and will not chain console commands. A bare
    /// all-digit string is treated as a workshop id (host_workshop_map).
    /// </summary>
    public static string ChangeCommandFor(string name)
    {
        name = name.Trim();
        var m = Resolve(name);
        if (m != null) return ChangeCommand(m);
        if (name.Length > 0 && name.All(char.IsDigit)) return $"host_workshop_map {name}";
        return $"changelevel {name}";
    }
}
