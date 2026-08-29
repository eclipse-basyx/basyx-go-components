# Pull Request Template

## Description of Changes

Please provide a brief summary of what you have changed in this pull request. Be clear and concise.

## Changelog

Select exactly one option:

- [ ] I added a Changie fragment under `.changes/unreleased` for every notable user-visible or security-related change.
- [ ] This pull request has no notable user-visible or security effect; a reviewer may apply the `no-changelog` label.

Open this pull request as a draft to obtain its number, then create fragments
interactively with Changie v1.26.0 by running `changie new`. Classify the user
impact as `High` when users must update an API integration, configuration,
policy, or deployment; otherwise use `Low`. Always describe the security
consequence, or enter `None.` when there is none. `CHANGELOG.md` is generated
during releases and must not be edited directly.

## Related Issue

If this pull request is related to an existing issue, please link that issue using the format `#issue_number`.

## BaSyx Configuration for Testing

Describe any specific configuration settings or environment setup required to test these changes effectively. If possible, please provide the BaSyx configuration as .zip file.

## AAS Files Used for Testing

Please provide any AAS files that were used in the testing of these changes. Include a brief descriptions of their relevance to the changes being proposed.

## Additional Information

Include any additional information or context that could be useful for the reviewer. This could include challenges faced, alternative solutions that were considered, etc.

---

Please ensure that you have tested your changes thoroughly before submitting the pull request.
