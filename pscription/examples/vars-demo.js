// Demonstrates that a script receives the pscription's vars as its first
// argument. The credential-separation pattern uses exactly this: the login
// LOGIC lives here (committed), the SECRETS live in a JSON vars file
// (gitignored) that the pscription references — this script never contains
// the values it applies.
((vars) => {
    return {
        sawUsername: vars.username || null,
        sawGreeting: vars.greeting || null,
    };
})
