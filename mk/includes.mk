##
# The target for this file is just to enumerate which partial
# makefile we want to use to compose our final Makefile.
#
# Unless you are not using conditional assignment within
# the different variable files, this would be the priority:
# - The values indicated at 'configs/config.yaml' file.
# - The values indicated at 'mk/variables.mk' file. This
#   file is included into the repository and define the
#   default values for the variables, if not assigned yet.
# - The 'mk/meta-*.mk' files just contain the comment to
#   print out the group text for the help content. They
#   are into independent files, because the order they
#   appear into this include file matters, and provide
#   the flexibility to print out the group text exactly
#   where we want kust changing the order into this file.
#
# This file set the 'help' rule as the default one when
# no arguments are indicated.
##
include mk/projectdir.mk
include mk/variables.mk

include mk/help.mk
include mk/meta-general.mk
include mk/go-rules.mk
include mk/printvars.mk
include mk/plantuml.mk
include mk/swag.mk
include mk/lint.mk
include mk/meta-db.mk
include mk/db.mk
include mk/meta-docker.mk
include mk/docker.mk
include mk/meta-repos.mk
include mk/repos.mk
