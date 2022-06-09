##
# Entrypoint for the Makefile
#
# It is composed at mk/includes.mk by including
# small make files which provides all the necessary
# rules.
#
# Some considerations:
#
# - Variables customization can be
#   stored at '.env', 'mk/private.mk' files.
# - By default the 'help' rule is executed.
# - No parallel jobs are executed from the main Makefile,
#   so that multiple rules from the command line will be
#   executed in serial.
##

include mk/includes.mk

.NOT_PARALLEL:

# Set the default rule
.DEFAULT_GOAL := help
