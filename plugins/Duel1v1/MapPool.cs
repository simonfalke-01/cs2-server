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
    // Workshop ids are popular community 1v1/aim maps; verify/replace as desired.
    public static readonly MapEntry[] All =
    {
        new("aim_map",        "aim_map (workshop)",      3070253702),
        new("aim_botz",       "aim_botz (workshop)",     3084291314),
        new("1v1_arena",      "1v1 Arena (workshop)",    3340432449),
        new("aim_redline",    "aim_redline (workshop)",  3071005299),
        // Stock maps (no workshop id -> changelevel).
        new("de_dust2",       "Dust II",                 0),
        new("de_mirage",      "Mirage",                  0),
        new("de_inferno",     "Inferno",                 0),
    };

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

    /// <summary>Change command for an arbitrary, possibly off-pool, map name.</summary>
    public static string ChangeCommandFor(string name)
    {
        var m = Resolve(name);
        if (m != null) return ChangeCommand(m);
        return $"changelevel {name}";
    }
}
