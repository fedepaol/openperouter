# Makefile for OpenPERouter VPN Setup Deployment

.PHONY: help install uninstall check tarball clean

help:
	@echo "OpenPERouter VPN Setup - Deployment Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  install      Install VPN setup on this system (requires root)"
	@echo "  uninstall    Uninstall VPN setup from this system (requires root)"
	@echo "  check        Verify all required files are present"
	@echo "  tarball      Create distribution tarball"
	@echo "  clean        Remove generated tarball"
	@echo ""
	@echo "Usage:"
	@echo "  sudo make install     # Install on current system"
	@echo "  make tarball          # Create tarball for distribution"
	@echo "  make check            # Verify package is complete"

install:
	@./install.sh

uninstall:
	@./uninstall.sh

check:
	@echo "Checking deployment package..."
	@for file in \
		usr/local/bin/setup-underlay.sh \
		usr/local/bin/setup-network.sh \
		usr/local/bin/generate-config.sh \
		usr/local/bin/common.sh \
		etc/systemd/system/setup-underlay.service \
		etc/systemd/system/setup-network.service \
		etc/systemd/system/generate-config.service \
		etc/openperouter/templates/openpe_evpn.yaml.template \
		etc/openperouter/vpn-setup.env.example \
		install.sh \
		uninstall.sh \
		README.md; \
	do \
		if [ -f "$$file" ]; then \
			echo "  ✓ $$file"; \
		else \
			echo "  ✗ $$file MISSING"; \
			exit 1; \
		fi; \
	done
	@echo ""
	@echo "All files present!"

tarball: check
	@echo "Creating distribution tarball..."
	@tar czf openperouter-vpn-setup.tar.gz \
		--transform 's,^,openperouter-vpn-setup/,' \
		usr/ etc/ *.sh README.md Makefile
	@echo "  ✓ Created openperouter-vpn-setup.tar.gz"
	@ls -lh openperouter-vpn-setup.tar.gz

clean:
	@rm -f openperouter-vpn-setup.tar.gz
	@echo "Cleaned tarball"
