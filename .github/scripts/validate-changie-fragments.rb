#!/usr/bin/env ruby
# /*******************************************************************************
# * Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
# *
# * Permission is hereby granted, free of charge, to any person obtaining
# * a copy of this software and associated documentation files (the
# * "Software"), to deal in the Software without restriction, including
# * without limitation the rights to use, copy, modify, merge, publish,
# * distribute, sublicense, and/or sell copies of the Software, and to
# * permit persons to whom the Software is furnished to do so, subject to
# * the following conditions:
# *
# * The above copyright notice and this permission notice shall be
# * included in all copies or substantial portions of the Software.
# *
# * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
# * LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
# * OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
# * WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
# *
# * SPDX-License-Identifier: MIT
# ******************************************************************************/

require "date"
require "yaml"

ALLOWED_KINDS = %w[added changed deprecated removed fixed security].freeze
EXPECTED_FIELDS = %w[kind body time custom].freeze
EXPECTED_CUSTOM_FIELDS = %w[Impact PullRequest SecurityImpact].freeze

def report(file, message)
  warn "::error title=CHLOG-GUARD-VALIDATEFRAGMENT::#{file}: #{message}"
  false
end

def valid_fragment?(file)
  fragment = YAML.safe_load(File.read(file), permitted_classes: [Date, Time], aliases: false)
  unless fragment.is_a?(Hash) && fragment.keys.sort == EXPECTED_FIELDS.sort
    return report(file, "expected exactly the fields #{EXPECTED_FIELDS.join(', ')}")
  end
  custom = fragment["custom"]
  unless custom.is_a?(Hash) && custom.keys.sort == EXPECTED_CUSTOM_FIELDS.sort
    return report(file, "expected exactly the custom fields #{EXPECTED_CUSTOM_FIELDS.join(', ')}")
  end

  checks = [
    [ALLOWED_KINDS.include?(fragment["kind"]), "kind must be one of #{ALLOWED_KINDS.join(', ')}"],
    [fragment["body"].is_a?(String) && !fragment["body"].strip.empty?, "body must not be blank"],
    [fragment["time"].is_a?(Time), "time must be an ISO-8601 timestamp"],
    [%w[High Low].include?(custom["Impact"]), "Impact must be High or Low"],
    [custom["PullRequest"].is_a?(Integer) && custom["PullRequest"].positive?, "PullRequest must be a positive integer"],
    [custom["SecurityImpact"].is_a?(String) && !custom["SecurityImpact"].strip.empty?, "SecurityImpact must not be blank"]
  ]
  checks.all? { |passed, message| passed || report(file, message) }
rescue Psych::Exception => error
  report(file, "invalid YAML: #{error.message}")
end

fragments = Dir[".changes/unreleased/*.{yaml,yml}"].sort
exit 1 unless fragments.map { |file| valid_fragment?(file) }.all?
