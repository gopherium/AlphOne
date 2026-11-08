Feature: A plugin keeps the files of each workspace apart
  A plugin that needs to keep files, such as the attachments of a mail,
  saves them through the file store AlphOne hands it. A file belongs to
  the workspace that saved it and to the plugin that saved it, so no
  other workspace or plugin can open it or delete it. A plugin may choose
  the id of a file itself, so it can write the id down before the file is
  stored. Files live in PostgreSQL unless the server is set up for an S3
  bucket. A plugin reports its failures through the server log AlphOne
  hands it.

  Background:
    Given the settings point at a migrated database
    And the workspace "Acme" exists
    And the workspace "Globex" exists

  Scenario: A plugin keeps each workspace's files apart
    Given AlphOne runs a plugin that keeps files
    When the workspace "Acme" saves the file "Receipt 184467235 for Maria Perez"
    Then the workspace "Globex" cannot open that file
    And the workspace "Globex" cannot delete that file
    And the workspace "Acme" still opens that file as "Receipt 184467235 for Maria Perez"

  Scenario: A file over the cap is not stored
    Given the files hold at most 16 bytes
    And AlphOne runs a plugin that keeps files
    When the workspace "Acme" saves a file of 17 bytes
    Then the save is refused as too large
    And no file is stored

  Scenario: A plugin saves a file under an id it chose
    Given AlphOne runs a plugin that keeps files
    When the workspace "Acme" saves the file "Receipt 184467235 for Maria Perez" under an id it chose
    Then the file is stored under the id the plugin chose
    And the workspace "Acme" still opens that file as "Receipt 184467235 for Maria Perez"
    And the workspace "Globex" cannot open that file

  Scenario: A second save under the same id answers an error
    Given AlphOne runs a plugin that keeps files
    And the workspace "Acme" saved the file "Receipt 184467235 for Maria Perez" under an id it chose
    When the workspace "Acme" saves the file "Refund for Maria Perez" under the same id
    Then the save is refused as an id in use
    And the workspace "Acme" still opens that file as "Receipt 184467235 for Maria Perez"

  Scenario: A plugin reports a failure in the server log
    Given AlphOne runs a plugin that keeps files
    When the plugin reports the failure "could not delete an old file"
    Then the server log holds the error "could not delete an old file"
