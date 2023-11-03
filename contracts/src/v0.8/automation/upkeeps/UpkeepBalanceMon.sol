// SPDX-License-Identifier: MIT

pragma solidity 0.8.6;

import "../../shared/access/ConfirmedOwner.sol";
import {IKeeperRegistryMaster} from "../interfaces/v2_1/IKeeperRegistryMaster.sol";
import {LinkTokenInterface} from "../../shared/interfaces/LinkTokenInterface.sol";
import {ITypeAndVersion} from "../../shared/interfaces/ITypeAndVersion.sol";
import "@openzeppelin/contracts/security/Pausable.sol";

/// @title The UpkeepBalanceMonitor contract.
/// @notice A keeper-compatible contract that monitors and funds Chainlink Automation upkeeps.
contract UpkeepBalanceMonitor is ConfirmedOwner, Pausable {
  uint256 private constant MIN_GAS_FOR_TRANSFER = 55_000;
  LinkTokenInterface public immutable LINK_TOKEN;

  event FundsAdded(uint256 amountAdded, uint256 newBalance, address sender);
  event FundsWithdrawn(uint256 amountWithdrawn, address payee);
  event KeeperRegistryAddressUpdated(IKeeperRegistryMaster oldAddress, IKeeperRegistryMaster newAddress);
  event LinkTokenAddressUpdated(address oldAddress, address newAddress);
  event MinWaitPeriodUpdated(uint256 oldMinWaitPeriod, uint256 newMinWaitPeriod);
  event OutOfGas(uint256 lastId);
  event TopUpFailed(uint256 indexed upkeepId);
  event TopUpSucceeded(uint256 indexed upkeepId);

  error DuplicateSubcriptionId(uint256 duplicate);
  error InvalidKeeperRegistryVersion();
  error InvalidWatchList();
  error OnlyKeeperRegistry();

  struct Target {
    bool isActive;
    uint96 minBalanceJuels;
    uint96 topUpAmountJuels;
    uint56 lastTopUpTimestamp;
  }

  IKeeperRegistryMaster private s_registry;
  uint256 private s_minWaitPeriodSeconds; // minimum time to wait between top-ups
  uint256[] private s_watchList; // the watchlist on which subscriptions are stored
  mapping(uint256 => Target) private s_targets;

  /// @param linkTokenAddress the Link token address
  /// @param keeperRegistryAddress the address of the keeper registry contract
  /// @param minWaitPeriodSeconds the minimum wait period for addresses between funding
  constructor(
    address linkTokenAddress,
    IKeeperRegistryMaster keeperRegistryAddress,
    uint256 minWaitPeriodSeconds
  ) ConfirmedOwner(msg.sender) {
    require(linkTokenAddress != address(0));
    if (keccak256(bytes(keeperRegistryAddress.typeAndVersion())) != keccak256(bytes("KeeperRegistry 2.1.0"))) {
      revert InvalidKeeperRegistryVersion();
    }
    LINK_TOKEN = LinkTokenInterface(linkTokenAddress);
    setKeeperRegistryAddress(keeperRegistryAddress);
    setMinWaitPeriodSeconds(minWaitPeriodSeconds);
    LinkTokenInterface(linkTokenAddress).approve(address(keeperRegistryAddress), type(uint256).max);
  }

  /// @notice Sets the list of upkeeps to watch and their funding parameters.
  /// @param upkeepIDs the list of subscription ids to watch
  /// @param minBalancesJuels the minimum balances for each upkeep
  /// @param topUpAmountsJuels the amount to top up each upkeep
  function setWatchList(
    uint256[] calldata upkeepIDs,
    uint96[] calldata minBalancesJuels,
    uint96[] calldata topUpAmountsJuels
  ) external onlyOwner {
    if (upkeepIDs.length != minBalancesJuels.length || upkeepIDs.length != topUpAmountsJuels.length) {
      revert InvalidWatchList();
    }
    uint256[] memory oldWatchList = s_watchList;
    for (uint256 i = 0; i < oldWatchList.length; i++) {
      s_targets[oldWatchList[i]].isActive = false;
    }
    for (uint256 i = 0; i < upkeepIDs.length; i++) {
      if (s_targets[upkeepIDs[i]].isActive) {
        revert DuplicateSubcriptionId(upkeepIDs[i]);
      }
      if (upkeepIDs[i] == 0 || topUpAmountsJuels[i] == 0) {
        revert InvalidWatchList();
      }
      s_targets[upkeepIDs[i]] = Target({
        isActive: true,
        minBalanceJuels: minBalancesJuels[i],
        topUpAmountJuels: topUpAmountsJuels[i],
        lastTopUpTimestamp: 0
      });
    }
    s_watchList = upkeepIDs;
  }

  /// @notice Gets a list of upkeeps that are underfunded.
  /// @return list of upkeeps that are underfunded
  function getUnderfundedUpkeeps() public view returns (uint256[] memory) {
    uint256 numUpkeeps = s_watchList.length;
    uint256[] memory needsFunding = new uint256[](numUpkeeps);
    uint256 minWaitPeriod = s_minWaitPeriodSeconds;
    uint256 contractBalance = LINK_TOKEN.balanceOf(address(this));
    uint256 count;
    Target memory target;
    uint256 upkeepID;
    for (uint256 i = 0; i < numUpkeeps; i++) {
      upkeepID = s_watchList[i];
      target = s_targets[upkeepID];
      uint96 upkeepBalance = s_registry.getBalance(upkeepID);
      uint96 minUpkeepBalance = s_registry.getMinBalance(upkeepID);
      uint96 minBalanceWithBuffer = getBalanceWithBuffer(minUpkeepBalance);
      if (
        target.lastTopUpTimestamp + minWaitPeriod <= block.timestamp &&
        contractBalance >= target.topUpAmountJuels &&
        (upkeepBalance < target.minBalanceJuels ||
          //upkeepBalance < minUpkeepBalance)
          upkeepBalance < minBalanceWithBuffer)
      ) {
        needsFunding[count] = upkeepID;
        count++;
        contractBalance -= target.topUpAmountJuels;
      }
    }
    if (count < numUpkeeps) {
      assembly {
        mstore(needsFunding, count)
      }
    }
    return needsFunding;
  }

  /// @notice Send funds to the upkeeps provided.
  /// @param needsFunding the list of upkeeps to fund
  function topUp(uint256[] memory needsFunding) public whenNotPaused {
    uint256 minWaitPeriodSeconds = s_minWaitPeriodSeconds;
    uint256 contractBalance = LINK_TOKEN.balanceOf(address(this));
    Target memory target;
    for (uint256 i = 0; i < needsFunding.length; i++) {
      target = s_targets[needsFunding[i]];
      uint96 upkeepBalance = s_registry.getBalance(needsFunding[i]);
      uint96 minUpkeepBalance = s_registry.getMinBalanceForUpkeep(needsFunding[i]);
      uint96 minBalanceWithBuffer = getBalanceWithBuffer(minUpkeepBalance);
      if (
        target.isActive &&
        target.lastTopUpTimestamp + minWaitPeriodSeconds <= block.timestamp &&
        (upkeepBalance < target.minBalanceJuels ||
          //upkeepBalance < minUpkeepBalance) &&
          upkeepBalance < minBalanceWithBuffer) &&
        contractBalance >= target.topUpAmountJuels
      ) {
        s_registry.addFunds(needsFunding[i], target.topUpAmountJuels);
        s_targets[needsFunding[i]].lastTopUpTimestamp = uint56(block.timestamp);
        contractBalance -= target.topUpAmountJuels;
        emit TopUpSucceeded(needsFunding[i]);
      }
      if (gasleft() < MIN_GAS_FOR_TRANSFER) {
        emit OutOfGas(i);
        return;
      }
    }
  }

  /// @notice Gets list of upkeeps ids that are underfunded and returns a keeper-compatible payload.
  /// @return upkeepNeeded signals if upkeep is needed, performData is an abi encoded list of subscription ids that need funds
  function checkUpkeep(
    bytes calldata
  ) external view whenNotPaused returns (bool upkeepNeeded, bytes memory performData) {
    uint256[] memory needsFunding = getUnderfundedUpkeeps();
    upkeepNeeded = needsFunding.length > 0;
    performData = abi.encode(needsFunding);
    return (upkeepNeeded, performData);
  }

  /// @notice Called by the keeper to send funds to underfunded addresses.
  /// @param performData the abi encoded list of addresses to fund
  function performUpkeep(bytes calldata performData) external whenNotPaused {
    // if (msg.sender != address(s_registry)) revert OnlyKeeperRegistry();
    // TODO - forwarder contract
    uint256[] memory needsFunding = abi.decode(performData, (uint256[]));
    topUp(needsFunding);
  }

  /// @notice Withdraws the contract balance in LINK.
  /// @param amount the amount of LINK (in juels) to withdraw
  /// @param payee the address to pay
  function withdraw(uint256 amount, address payee) external onlyOwner {
    require(payee != address(0));
    LINK_TOKEN.transfer(payee, amount);
    emit FundsWithdrawn(amount, payee);
  }

  /// @notice Sets the keeper registry address.
  function setKeeperRegistryAddress(IKeeperRegistryMaster keeperRegistryAddress) public onlyOwner {
    require(address(keeperRegistryAddress) != address(0));
    s_registry = keeperRegistryAddress;
    emit KeeperRegistryAddressUpdated(s_registry, keeperRegistryAddress);
  }

  /// @notice Sets the minimum wait period (in seconds) for upkeep ids between funding.
  function setMinWaitPeriodSeconds(uint256 period) public onlyOwner {
    s_minWaitPeriodSeconds = period;
    emit MinWaitPeriodUpdated(s_minWaitPeriodSeconds, period);
  }

  /// @notice Gets configuration information for a upkeep on the watchlist.
  function getUpkeepInfo(
    uint256 upkeepId
  ) external view returns (bool isActive, uint96 minBalanceJuels, uint96 topUpAmountJuels, uint56 lastTopUpTimestamp) {
    Target memory target = s_targets[upkeepId];
    return (target.isActive, target.minBalanceJuels, target.topUpAmountJuels, target.lastTopUpTimestamp);
  }

  /// @notice Gets the keeper registry address
  function getKeeperRegistryAddress() external view returns (address) {
    return address(s_registry);
  }

  /// @notice Gets the minimum wait period (in seconds) for upkeep ids between funding.
  function getMinWaitPeriodSeconds() external view returns (uint256) {
    return s_minWaitPeriodSeconds;
  }

  /// @notice Gets the list of upkeeps ids being watched.
  function getWatchList() external view returns (uint256[] memory) {
    return s_watchList;
  }

  /// @notice Pause the contract, which prevents executing performUpkeep.
  function pause() external onlyOwner {
    _pause();
  }

  /// @notice Unpause the contract.
  function unpause() external onlyOwner {
    _unpause();
  }

  /// @notice Called to add buffer to minimum balance of upkeeps
  /// @param num the current minimum balance
  function getBalanceWithBuffer(uint96 num) private pure returns (uint96) {
    uint96 buffer = 20;
    uint96 result = uint96((uint256(num) * (100 + buffer)) / 100); // convert to uint256 to prevent overflow
    return result;
  }
}
