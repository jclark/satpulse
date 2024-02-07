Name: satpulse
Version: %{version}
Release: 1
Summary: Use a GPS receiver to provide precision time to PTP and NTP daemons
License: MIT
URL: https://github.com/jclark/satpulse

%global _build_id_links none
%global source_date_epoch_from_changelog 0

%description
This requires an ethernet controller with a PPS input pin that is supported by
the kernel's PTP hardware clock infrastructure, such as the Intel i210
or the Raspberry Pi CM4.

%install
rm -rf %{buildroot}
install -D out/%{goarch}/satpulsed %{buildroot}%{_sbindir}/satpulsed
install -D out/%{goarch}/satpulsetool %{buildroot}%{_bindir}/satpulsetool
install -D -m 644 out/%{goarch}/default.toml %{buildroot}%{_sysconfdir}/satpulse/default.toml
install -D -m 644 configs/chrony.conf %{buildroot}%{_docdir}/satpulse/chrony.conf
install -D -m 644 LICENSE %{buildroot}%{_docdir}/satpulse/copyright
install -D -m 644 out/satpulse@.service %{buildroot}%{_unitdir}/satpulse@.service

%files
%{_sbindir}/satpulsed
%{_bindir}/satpulsetool
%dir %{_sysconfdir}/satpulse
%config(noreplace) %{_sysconfdir}/satpulse/default.toml
%{_unitdir}/satpulse@.service
%{_docdir}/satpulse/chrony.conf
%{_docdir}/satpulse/copyright
