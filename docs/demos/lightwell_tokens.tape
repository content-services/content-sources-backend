# Lightwell access tokens — VHS cassette
#
# Render via: REPO_UUID=<uuid> ./scripts/render_lightwell_tokens_demo.sh
# Requires: vhs, ttyd, ffmpeg; local CS API (make run) with a seeded Lightwell repo.

Output docs/demos/lightwell_tokens.gif
Output docs/demos/lightwell_tokens.mp4

Set Shell "bash"
Set FontSize 22
Set Width 1280
Set Height 720
Set TypingSpeed 45ms
Set Framerate 30
Set PlaybackSpeed 1
Set WindowBar Colorful
Set Padding 16
Set Theme "Catppuccin Mocha"

# Bootstrap env written by scripts/render_lightwell_tokens_demo.sh
# (file includes cd to the repo root).
Hide
Type "set -a && source docs/demos/.lightwell_demo_env && set +a"
Enter
Type "clear"
Enter
Show

Type "./scripts/demo_lightwell_tokens.sh"
Enter
# Script ~35s at STEP_PAUSE=2 (intro + extra deny/allow beats); hold on Done.
Sleep 42s
