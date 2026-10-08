# Large Install Simulator

This offline demonstration package deliberately takes about **40 seconds** to install.

It emits a new progress line every two seconds so the CodeStation Packages installation window can be checked for:

- live log streaming;
- automatic scrolling;
- readable OpenTofu provisioner output;
- correct completion status;
- progressive uninstall output.

The package only creates a small file under `~/.local/share/codestation-large-install` and never accesses the Internet.
