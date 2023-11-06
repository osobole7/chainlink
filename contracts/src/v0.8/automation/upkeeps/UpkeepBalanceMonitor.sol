// SPDX-License-Identifier: MIT

pragma solidity 0.8.6;

import {ConfirmedOwner} from "../../shared/access/ConfirmedOwner.sol";
import {IAutomationForwarder} from "../interfaces/IAutomationForwarder.sol";
import {IAutomationRegistryConsumer} from "../interfaces/IAutomationRegistryConsumer.sol";
import {LinkTokenInterface} from "../../shared/interfaces/LinkTokenInterface.sol";
import {Pausable} from "@openzeppelin/contracts/security/Pausable.sol";

/// @title The UpkeepBalanceMonitor contract
/// @notice A keeper-compatible contract that monitors and funds Chainlink Automation upkeeps.
contract UpkeepBalanceMonitor is ConfirmedOwner, Pausable {
  event ConfigSet(Config config);
  event ForwarderSet(IAutomationForwarder forwarder);
  event FundsWithdrawn(uint256 amountWithdrawn, address payee);
  event TopUpFailed(uint256 indexed upkeepId);
  event TopUpSucceeded(uint256 indexed upkeepId, uint96 amount);
  event WatchListSet();

  error InvalidConfig();
  error InvalidTopUpData();
  error OnlyForwarderOrOwner();

  /// @member maxBatchSize is the maximum number of upkeeps to fund in a single transaction
  /// @member minPercentage is the percentage of the upkeep's minBalance at which top-up occurs
  /// @member targetPercentage is the percentage of the upkeep's minBalance to top-up to
  /// @member maxTopUpAmount is the maximum amount of LINK to top-up an upkeep with
  struct Config {
    uint8 maxBatchSize;
    uint24 minPercentage;
    uint24 targetPercentage;
    uint96 maxTopUpAmount;
  }

  // ================================================================
  // |                           STORAGE                            |
  // ================================================================

  LinkTokenInterface private immutable LINK_TOKEN;

  uint256[] private s_watchList;
  Config private s_config;
  IAutomationForwarder private s_forwarder;

  // ================================================================
  // |                         CONSTRUCTOR                          |
  // ================================================================

  /// @param linkToken the Link token address
  /// @param config the initial config for the contract
  constructor(LinkTokenInterface linkToken, Config memory config) ConfirmedOwner(msg.sender) {
    require(address(linkToken) != address(0));
    LINK_TOKEN = linkToken;
    setConfig(config);
  }

  // ================================================================
  // |                    AUTOMATION COMPATIBLE                     |
  // ================================================================

  /// @notice Gets list of upkeeps ids that are underfunded and returns a keeper-compatible payload.
  /// @return upkeepNeeded signals if upkeep is needed, performData is an abi encoded list of subscription ids that need funds
  function checkUpkeep(
    bytes calldata
  ) external view whenNotPaused returns (bool upkeepNeeded, bytes memory performData) {
    (uint256[] memory needsFunding, uint256[] memory topUpAmounts) = getUnderfundedUpkeeps();
    upkeepNeeded = needsFunding.length > 0;
    if (upkeepNeeded) {
      performData = abi.encode(needsFunding, topUpAmounts);
    }
    return (upkeepNeeded, performData);
  }

  /// @notice Called by the keeper to send funds to underfunded addresses.
  /// @param performData the abi encoded list of addresses to fund
  function performUpkeep(bytes calldata performData) external whenNotPaused {
    (uint256[] memory upkeepIDs, uint96[] memory topUpAmounts) = abi.decode(performData, (uint256[], uint96[]));
    topUp(upkeepIDs, topUpAmounts);
  }

  /// @notice Called by the keeper/owner to send funds to underfunded upkeeps
  /// @param upkeepIDs the list of upkeep ids to fund
  /// @param topUpAmounts the list of amounts to fund each upkeep with
  /// @dev We explicitly choose not to verify that input upkeepIDs are included in the watchlist. We also
  /// explicity permit any amount to be sent via topUpAmounts; it does not have to meet the criteria
  /// specified in getUnderfundedUpkeeps(). Here, we are relying on the security of automation's OCR to
  /// secure the output of getUnderfundedUpkeeps() as the input to topUp(), and we are treating the owner
  /// as a privileged user that can perform arbitrary top-ups to any upkeepID.
  function topUp(uint256[] memory upkeepIDs, uint96[] memory topUpAmounts) public {
    IAutomationForwarder forwarder = s_forwarder;
    if (msg.sender != address(s_forwarder) && msg.sender != owner()) revert OnlyForwarderOrOwner();
    if (upkeepIDs.length != topUpAmounts.length) revert InvalidTopUpData();
    address registryAddress = address(forwarder.getRegistry());
    for (uint256 i = 0; i < upkeepIDs.length; i++) {
      try LINK_TOKEN.transferAndCall(registryAddress, topUpAmounts[i], abi.encode(upkeepIDs[i])) returns (
        bool success
      ) {
        if (success) {
          emit TopUpSucceeded(upkeepIDs[i], topUpAmounts[i]);
          continue;
        }
      } catch {}
      emit TopUpFailed(upkeepIDs[i]);
    }
  }

  // ================================================================
  // |                            ADMIN                             |
  // ================================================================

  /// @notice Withdraws the contract balance in LINK.
  /// @param amount the amount of LINK (in juels) to withdraw
  /// @param payee the address to pay
  function withdraw(uint256 amount, address payee) external onlyOwner {
    require(payee != address(0));
    LINK_TOKEN.transfer(payee, amount);
    emit FundsWithdrawn(amount, payee);
  }

  /// @notice Pause the contract, which prevents executing performUpkeep.
  function pause() external onlyOwner {
    _pause();
  }

  /// @notice Unpause the contract.
  function unpause() external onlyOwner {
    _unpause();
  }

  // ================================================================
  // |                           SETTERS                            |
  // ================================================================

  /// @notice Sets the list of upkeeps to watch and their funding parameters.
  /// @param watchlist the list of subscription ids to watch
  function setWatchList(uint256[] calldata watchlist) external onlyOwner {
    s_watchList = watchlist;
    emit WatchListSet();
  }

  /// @notice Sets the contract config
  /// @param config the new config
  function setConfig(Config memory config) public onlyOwner {
    if (
      config.maxBatchSize == 0 ||
      config.minPercentage < 100 ||
      config.targetPercentage <= config.minPercentage ||
      config.maxTopUpAmount == 0
    ) {
      revert InvalidConfig();
    }
    s_config = config;
    emit ConfigSet(config);
  }

  /// @notice Sets the upkeep's forwarder contract
  /// @param forwarder the new forwarder
  /// @dev this should only need to be called once, after registering the contract with the registry
  function setForwarder(IAutomationForwarder forwarder) external onlyOwner {
    s_forwarder = forwarder;
    emit ForwarderSet(forwarder);
  }

  // ================================================================
  // |                           GETTERS                            |
  // ================================================================

  /// @notice Gets a list of upkeeps that are underfunded.
  /// @return needsFunding list of underfunded upkeepIDs
  /// @return topUpAmounts amount to top up each upkeep
  function getUnderfundedUpkeeps() public view returns (uint256[] memory, uint256[] memory) {
    uint256 numUpkeeps = s_watchList.length;
    uint256[] memory needsFunding = new uint256[](numUpkeeps);
    uint256[] memory topUpAmounts = new uint256[](numUpkeeps);
    Config memory config = s_config;
    IAutomationRegistryConsumer registry = getRegistry();
    uint256 availableFunds = LINK_TOKEN.balanceOf(address(this));
    uint256 count;
    uint256 upkeepID;
    for (uint256 i = 0; i < numUpkeeps; i++) {
      upkeepID = s_watchList[i];
      uint96 upkeepBalance = registry.getBalance(upkeepID);
      uint256 minBalance = uint256(registry.getMinBalance(upkeepID));
      uint256 topUpThreshold = (minBalance * config.minPercentage) / 100;
      uint256 topUpAmount = ((minBalance * config.targetPercentage) / 100) - upkeepBalance;
      if (topUpAmount > config.maxTopUpAmount) {
        topUpAmount = config.maxTopUpAmount;
      }
      if (upkeepBalance <= topUpThreshold && availableFunds >= topUpAmount) {
        needsFunding[count] = upkeepID;
        topUpAmounts[count] = topUpAmount;
        count++;
        availableFunds -= topUpAmount;
      }
      if (count == config.maxBatchSize) {
        break;
      }
    }
    if (count < numUpkeeps) {
      assembly {
        mstore(needsFunding, count)
        mstore(topUpAmounts, count)
      }
    }
    return (needsFunding, topUpAmounts);
  }

  /// @notice Gets the list of upkeeps ids being monitored
  function getWatchList() external view returns (uint256[] memory) {
    return s_watchList;
  }

  /// @notice Gets the contract config
  function getConfig() external view returns (Config memory) {
    return s_config;
  }

  /// @notice Gets the upkeep's forwarder contract
  function getForwarder() external view returns (IAutomationForwarder) {
    return s_forwarder;
  }

  /// @notice Gets the registry contract
  function getRegistry() public view returns (IAutomationRegistryConsumer) {
    return s_forwarder.getRegistry();
  }
}
